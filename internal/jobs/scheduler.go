package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/robfig/cron/v3"
)

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// NextRun is the first time after `after` that schedule fires in tz.
func NextRun(schedule, tz string, after time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	sched, err := cronParser.Parse(strings.TrimSpace(schedule))
	if err != nil {
		return time.Time{}, fmt.Errorf("the schedule %q isn't a valid cron expression (minute hour day month weekday)", schedule)
	}
	next := sched.Next(after.In(loc))
	if next.IsZero() {
		return next, errors.New("the schedule never fires")
	}
	return next, nil
}

// RunScheduler enqueues due searches every minute, sends the daily digest
// and schedules session checks. Several servers may run it: due rows are
// claimed with SKIP LOCKED.
func (s *Service) RunScheduler(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		if err := s.scheduleOnce(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("job scheduler", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s *Service) scheduleOnce(ctx context.Context) error {
	set, err := s.Repo.Settings(ctx)
	if err != nil {
		return err
	}
	now := time.Now()

	err = pgx.BeginFunc(ctx, s.Repo.db, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, schedule, portals, next_run_at IS NOT NULL FROM job_searches
			WHERE enabled AND (next_run_at IS NULL OR next_run_at <= now()) FOR UPDATE SKIP LOCKED`)
		if err != nil {
			return err
		}
		type due struct {
			id       int64
			schedule string
			portals  []string
			run      bool
		}
		var list []due
		for rows.Next() {
			var d due
			if err := rows.Scan(&d.id, &d.schedule, &d.portals, &d.run); err != nil {
				return err
			}
			list = append(list, d)
		}
		rows.Close()
		for _, d := range list {
			var next *time.Time
			if t, err := NextRun(d.schedule, set.Timezone, now); err == nil {
				next = &t
			}
			if _, err := tx.Exec(ctx, `UPDATE job_searches SET next_run_at=$2 WHERE id=$1`, d.id, next); err != nil {
				return err
			}
			// A search with no next_run_at was just created or re-enabled:
			// schedule it without running it immediately.
			if !d.run || set.Paused {
				continue
			}
			for _, p := range d.portals {
				id := d.id
				if _, err := tx.Exec(ctx, `INSERT INTO job_tasks (kind, search_id, portal, trigger)
					SELECT 'search', $1, $2, 'schedule' WHERE NOT EXISTS (SELECT 1 FROM job_tasks
						WHERE kind='search' AND search_id=$1 AND portal=$2 AND status IN ('pending','running'))`, id, p); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Daily digest at the configured hour.
	local := now.In(set.Location())
	if set.NotifyDigest && s.Bot() != nil && local.Hour() == set.DigestHour {
		if ok, err := s.Repo.ClaimDigest(ctx, local); err == nil && ok {
			s.SendDigest(ctx)
		}
	}

	// Session checks twice a day for connected accounts.
	accounts, err := s.Repo.Accounts(ctx)
	if err != nil {
		return err
	}
	for _, a := range accounts {
		if a.Enabled && a.Status == "connected" && (a.CheckedAt == nil || time.Since(*a.CheckedAt) > 12*time.Hour) {
			if _, err := s.Repo.Enqueue(ctx, "check", nil, a.Portal, "schedule"); err != nil {
				return err
			}
		}
	}
	return nil
}

// RunSearch queues a search on each of its portals now.
func (s *Service) RunSearch(ctx context.Context, id int64, trigger string) (int, error) {
	srch, err := s.Repo.Search(ctx, id)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range srch.Portals {
		ok, err := s.Repo.Enqueue(ctx, "search", &srch.ID, p, trigger)
		if err != nil {
			return n, err
		}
		if ok {
			n++
		}
	}
	return n, nil
}

func (s *Service) RunAllSearches(ctx context.Context, trigger string) (int, error) {
	list, err := s.Repo.Searches(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, srch := range list {
		if !srch.Enabled {
			continue
		}
		n, err := s.RunSearch(ctx, srch.ID, trigger)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}
