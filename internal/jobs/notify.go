package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/devileya/my-portfolio/internal/telegram"
)

const maxMatchMessages = 8

var esc = telegram.Escape

func listingLine(l Listing) string {
	var b strings.Builder
	if l.Score != nil {
		fmt.Fprintf(&b, "<b>%d</b> · ", *l.Score)
	}
	fmt.Fprintf(&b, `<a href="%s">%s</a> — %s`, esc(l.URL), esc(l.Title), esc(orLabel(l.Company, "?")))
	meta := []string{PortalLabel[l.Portal]}
	for _, v := range []string{l.WorkMode, l.Location, l.Salary} {
		if v != "" {
			meta = append(meta, v)
		}
	}
	fmt.Fprintf(&b, "\n<i>%s</i>", esc(strings.Join(meta, " · ")))
	return b.String()
}

func reviewButtons(id int64) []telegram.Button {
	return []telegram.Button{
		{Text: "✅ Apply", Data: fmt.Sprintf("ap:%d", id)},
		{Text: "⏭ Skip", Data: fmt.Sprintf("sk:%d", id)},
	}
}

// notifyMatches sends the run's new matches: queued ones with review buttons,
// auto-approved ones as a heads-up.
func (s *Service) notifyMatches(ctx context.Context, srch Search, list []Listing) {
	if s.Bot() == nil || len(list) == 0 {
		return
	}
	set, err := s.Repo.Settings(ctx)
	if err != nil || !set.NotifyMatches {
		return
	}
	s.Bot().Notify(ctx, fmt.Sprintf("🔎 <b>%s</b>: %d new match(es).", esc(srch.Name), len(list)))
	for i, l := range list {
		if i == maxMatchMessages {
			more := fmt.Sprintf("…and %d more in the review queue.", len(list)-i)
			if u := s.link("/jobs/queue"); u != "" {
				more += fmt.Sprintf(` <a href="%s">Open queue</a>`, esc(u))
			}
			s.Bot().Notify(ctx, more)
			break
		}
		text := listingLine(l)
		if l.ScoreReason != "" {
			text += "\n" + esc(l.ScoreReason)
		}
		if l.Status == StatusApproved {
			s.Bot().Notify(ctx, "🤖 Auto-approved:\n"+text)
			continue
		}
		id, err := s.Bot().Send(ctx, text, reviewButtons(l.ID))
		if err != nil {
			slog.Warn("telegram match", "err", err)
			return
		}
		lid := l.ID
		_ = s.Repo.SaveTelegramRef(ctx, TelegramRef{MessageID: id, Kind: "review", ListingID: &lid})
	}
}

func (s *Service) notifyApplied(ctx context.Context, l Listing, app Application, res applyResult) {
	if s.Bot() == nil {
		return
	}
	set, err := s.Repo.Settings(ctx)
	if err != nil {
		return
	}
	line := listingLine(l)
	switch res.Status {
	case "submitted":
		if set.NotifyApplied {
			s.Bot().Notify(ctx, "✅ Applied\n"+line)
		}
	case "dry_run":
		if set.NotifyApplied {
			s.Bot().Notify(ctx, "🧪 Dry run filled the form (not submitted)\n"+line)
		}
	case "external":
		s.Bot().Notify(ctx, "↗️ Apply on the company site yourself\n"+line)
	case "needs_input":
		s.Bot().Notify(ctx, "❓ Needs your answers\n"+line+"\n"+esc(res.Note))
		for _, q := range res.Pending {
			s.askQuestion(ctx, l.ID, q)
		}
	case "failed":
		if l.Status == StatusFailed {
			s.Bot().Notify(ctx, "❌ Couldn't apply\n"+line+"\n"+esc(Clip(res.Note, 400)))
		}
	}
}

// askQuestion sends one pending question. Choices become buttons; free text
// is answered by replying to the message.
func (s *Service) askQuestion(ctx context.Context, listingID int64, q Question) {
	text := "❓ " + esc(q.Question)
	var rows [][]telegram.Button
	if len(q.Options) > 0 && len(q.Options) <= 8 {
		for i, o := range q.Options {
			rows = append(rows, []telegram.Button{{Text: Clip(o, 60), Data: fmt.Sprintf("qa:%d", i)}})
		}
	} else {
		text += "\n<i>Reply to this message with the answer.</i>"
		if len(q.Options) > 0 {
			text += "\nChoices: " + esc(strings.Join(q.Options, " / "))
		}
	}
	id, err := s.Bot().Send(ctx, text, rows...)
	if err != nil {
		return
	}
	_ = s.Repo.SaveTelegramRef(ctx, TelegramRef{MessageID: id, Kind: "question", ListingID: &listingID, QuestionKey: q.Key, Question: q.Question})
}

// AnswerQuestions saves answers to a listing's pending questions and, when
// none are left, puts the listing back in the apply queue.
func (s *Service) AnswerQuestions(ctx context.Context, listingID int64, answers []Answer) (bool, error) {
	for _, a := range answers {
		if strings.TrimSpace(a.Question) == "" || strings.TrimSpace(a.Answer) == "" {
			continue
		}
		if _, err := s.Repo.SaveAnswer(ctx, SavedAnswer{Question: a.Question, Answer: strings.TrimSpace(a.Answer), Source: "manual", Approved: true}); err != nil {
			return false, err
		}
	}
	app, err := s.Repo.LatestApplication(ctx, listingID)
	if err != nil {
		return false, err
	}
	bank, err := s.Repo.AnswerBank(ctx)
	if err != nil {
		return false, err
	}
	for _, q := range app.PendingQuestions {
		if q.Kind == "file" {
			continue
		}
		if a, ok := bank[QuestionKey(q.Question)]; !ok || a.Answer == "" {
			return false, nil
		}
	}
	l, err := s.Repo.Listing(ctx, listingID)
	if err != nil {
		return false, err
	}
	if l.Status != StatusNeedsInput {
		return false, nil
	}
	return true, s.Repo.SetListingStatus(ctx, listingID, StatusApproved, "Answers added; retrying.")
}

// ── Telegram commands ────────────────────────────────────────────────────────

func (s *Service) HandleTelegram(ctx context.Context, u telegram.Update) {
	if cb := u.CallbackQuery; cb != nil {
		s.handleCallback(ctx, cb)
		return
	}
	m := u.Message
	if m == nil {
		return
	}
	if m.ReplyToMessage != nil {
		s.handleReply(ctx, m)
		return
	}
	cmd := strings.ToLower(strings.Fields(m.Text + " x")[0])
	if i := strings.IndexByte(cmd, '@'); i > 0 {
		cmd = cmd[:i]
	}
	switch cmd {
	case "/status", "/start":
		s.Bot().Notify(ctx, s.statusText(ctx))
	case "/pause":
		_ = s.Repo.SetPaused(ctx, true)
		s.Bot().Notify(ctx, "⏸ Job Hunter paused. /resume to continue.")
	case "/resume":
		_ = s.Repo.SetPaused(ctx, false)
		s.Bot().Notify(ctx, "▶️ Job Hunter resumed.")
	case "/queue":
		list, total, err := s.Repo.Listings(ctx, ListingFilter{Statuses: []string{StatusQueued}, Limit: 5})
		if err != nil || total == 0 {
			s.Bot().Notify(ctx, "The review queue is empty.")
			return
		}
		s.Bot().Notify(ctx, fmt.Sprintf("%d job(s) waiting for review. Top %d:", total, len(list)))
		for _, l := range list {
			if id, err := s.Bot().Send(ctx, listingLine(l)+"\n"+esc(l.ScoreReason), reviewButtons(l.ID)); err == nil {
				lid := l.ID
				_ = s.Repo.SaveTelegramRef(ctx, TelegramRef{MessageID: id, Kind: "review", ListingID: &lid})
			}
		}
	case "/run":
		n, err := s.RunAllSearches(ctx, "telegram")
		if err != nil {
			s.Bot().Notify(ctx, "Couldn't start the searches: "+esc(err.Error()))
			return
		}
		s.Bot().Notify(ctx, fmt.Sprintf("🔎 Queued %d search run(s).", n))
	default:
		s.Bot().Notify(ctx, "Commands: /status, /queue, /run, /pause, /resume. Reply to a ❓ message to answer it.")
	}
}

func (s *Service) handleCallback(ctx context.Context, cb *telegram.Callback) {
	kind, arg, _ := strings.Cut(cb.Data, ":")
	switch kind {
	case "ap", "sk":
		id, _ := strconv.ParseInt(arg, 10, 64)
		l, err := s.Repo.Review(ctx, id, kind == "ap")
		if err != nil {
			s.Bot().Answer(ctx, cb.ID, Clip(err.Error(), 180))
			return
		}
		verb := "✅ Approved, applying soon"
		if kind == "sk" {
			verb = "⏭ Skipped"
		}
		s.Bot().Answer(ctx, cb.ID, verb)
		_ = s.Bot().Edit(ctx, cb.Message.MessageID, verb+"\n"+listingLine(l))
	case "qa":
		ref, err := s.Repo.TelegramRef(ctx, cb.Message.MessageID)
		if err != nil || ref.ListingID == nil {
			s.Bot().Answer(ctx, cb.ID, "That question has expired.")
			return
		}
		idx, _ := strconv.Atoi(arg)
		app, err := s.Repo.LatestApplication(ctx, *ref.ListingID)
		if err != nil {
			s.Bot().Answer(ctx, cb.ID, "That question has expired.")
			return
		}
		for _, q := range app.PendingQuestions {
			if q.Key == ref.QuestionKey && idx >= 0 && idx < len(q.Options) {
				s.saveTelegramAnswer(ctx, cb.Message.MessageID, ref, q.Options[idx])
				s.Bot().Answer(ctx, cb.ID, "Saved")
				return
			}
		}
		s.Bot().Answer(ctx, cb.ID, "That question has expired.")
	}
}

func (s *Service) handleReply(ctx context.Context, m *telegram.Message) {
	ref, err := s.Repo.TelegramRef(ctx, m.ReplyToMessage.MessageID)
	if err != nil || ref.Kind != "question" || ref.ListingID == nil {
		s.Bot().Notify(ctx, "Reply to a ❓ question message to answer it.")
		return
	}
	s.saveTelegramAnswer(ctx, m.ReplyToMessage.MessageID, ref, m.Text)
}

func (s *Service) saveTelegramAnswer(ctx context.Context, messageID int64, ref TelegramRef, answer string) {
	requeued, err := s.AnswerQuestions(ctx, *ref.ListingID, []Answer{{Question: ref.Question, Answer: answer}})
	if err != nil {
		s.Bot().Notify(ctx, "Couldn't save that answer: "+esc(err.Error()))
		return
	}
	_ = s.Bot().Edit(ctx, messageID, "✔️ "+esc(ref.Question)+"\n<b>"+esc(answer)+"</b>")
	if requeued {
		s.Bot().Notify(ctx, "All questions answered: the application is back in the queue.")
	}
}

func (s *Service) statusText(ctx context.Context) string {
	o, err := s.Repo.Overview(ctx)
	if err != nil {
		return "Status unavailable: " + esc(err.Error())
	}
	var b strings.Builder
	state := "running"
	if o.Settings.Paused {
		state = "paused"
	}
	if o.Settings.DryRun {
		state += ", dry run"
	}
	fmt.Fprintf(&b, "<b>Job Hunter</b> (%s)\n", state)
	if o.Settings.WorkerSeenAt != nil {
		fmt.Fprintf(&b, "Worker %s: %s (%s ago)\n", esc(o.Settings.WorkerName), esc(o.Settings.WorkerState),
			time.Since(*o.Settings.WorkerSeenAt).Round(time.Second))
	} else {
		b.WriteString("Worker: never seen\n")
	}
	fmt.Fprintf(&b, "Queue: %d to review, %d approved, %d need answers\n",
		o.ByStatus[StatusQueued], o.ByStatus[StatusApproved], o.ByStatus[StatusNeedsInput])
	total := 0
	var parts []string
	for _, p := range Portals {
		total += o.AppliedToday[p]
		if n := o.AppliedToday[p]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", PortalLabel[p], n))
		}
	}
	fmt.Fprintf(&b, "Applied today: %d %s · last 7 days: %d\n", total, esc(strings.Join(parts, ", ")), o.AppliedWeek)
	for _, a := range o.Accounts {
		icon := map[string]string{"connected": "🟢", "expired": "🟠", "challenge": "🛑", "error": "🔴"}[a.Status]
		if icon == "" {
			icon = "⚪️"
		}
		if !a.Enabled {
			icon = "⏸"
		}
		fmt.Fprintf(&b, "%s %s %s\n", icon, a.Label, esc(a.StatusNote))
	}
	return b.String()
}

// SendDigest posts the daily summary.
func (s *Service) SendDigest(ctx context.Context) {
	s.Bot().Notify(ctx, "🗓 Daily summary\n"+s.statusText(ctx))
}
