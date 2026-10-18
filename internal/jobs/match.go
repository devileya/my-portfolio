package jobs

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/devileya/my-portfolio/internal/ai"
)

// filterReason applies the search's hard rules. It returns why the listing
// was dropped, or "" to keep it. Unknown values (e.g. a portal that doesn't
// say whether a job is remote) never filter.
func (s *Service) filterReason(ctx context.Context, srch Search, l Listing) (string, error) {
	if hit := matchesAny(l.Title, srch.ExcludeKeywords); hit != "" {
		return fmt.Sprintf("Title contains excluded keyword %q.", hit), nil
	}
	for _, c := range srch.ExcludeCompanies {
		if c = strings.TrimSpace(c); c != "" && strings.Contains(strings.ToLower(l.Company), strings.ToLower(c)) {
			return fmt.Sprintf("Company %q is excluded.", l.Company), nil
		}
	}
	if len(srch.WorkModes) > 0 && l.WorkMode != "" && !containsFold(srch.WorkModes, l.WorkMode) {
		return fmt.Sprintf("Work mode is %s.", l.WorkMode), nil
	}
	if len(srch.JobTypes) > 0 && l.JobType != "" && !containsFold(srch.JobTypes, l.JobType) {
		return fmt.Sprintf("Job type is %s.", strings.ReplaceAll(l.JobType, "_", " ")), nil
	}
	if srch.EasyApplyOnly && l.EasyApply != nil && !*l.EasyApply {
		return "Applying happens on the company's own site.", nil
	}
	if srch.PostedWithinDays > 0 && l.PostedAt != nil && time.Since(*l.PostedAt) > time.Duration(srch.PostedWithinDays+1)*24*time.Hour {
		return fmt.Sprintf("Posted %s, older than %d days.", l.PostedAt.Format("2 Jan"), srch.PostedWithinDays), nil
	}
	if len(srch.Locations) > 0 && l.WorkMode != "remote" && l.Location != "" {
		ok := false
		for _, loc := range srch.Locations {
			if loc = strings.TrimSpace(loc); loc == "" || strings.Contains(strings.ToLower(l.Location), strings.ToLower(loc)) ||
				strings.EqualFold(loc, "remote") || strings.EqualFold(loc, "anywhere") {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Sprintf("Location %s is not in %s.", l.Location, strings.Join(srch.Locations, ", ")), nil
		}
	}
	if dup, err := s.Repo.DuplicateOf(ctx, l.ID, Fingerprint(l.Company, l.Title)); err != nil || dup {
		return "Already queued or applied from another portal.", err
	}
	if recent, err := s.Repo.RecentlyAppliedCompany(ctx, l.Company); err != nil || recent {
		return fmt.Sprintf("Applied to %s in the last 30 days.", l.Company), err
	}
	return "", nil
}

type scoreReply struct {
	Score    int      `json:"score"`
	Verdict  string   `json:"verdict"`
	Reason   string   `json:"reason"`
	Matched  []string `json:"matched"`
	Missing  []string `json:"missing"`
	Concerns []string `json:"concerns"`
}

const scoreSystem = `You screen job listings for one candidate and rate how well each fits them.

Score 0-100 for fit: skills and stack overlap, seniority and years of experience, role type, and the candidate's stated preferences. Guide:
90+ near-perfect fit; 75-89 strong, worth applying; 60-74 plausible with gaps; 40-59 weak; under 40 wrong role or level.
Penalize hard requirements the candidate clearly lacks (a must-have language, licence, degree or years far above theirs) and roles far below their level. Don't reward keyword overlap alone.
Fields:
- score: integer
- verdict: one of "strong", "good", "weak", "poor"
- reason: one or two plain sentences a busy person can scan
- matched: up to 6 short items the candidate clearly has that the job wants
- missing: up to 6 short items the job wants that the candidate lacks or that aren't evident
- concerns: up to 3 short red flags (unclear pay, agency spam, mismatch in location, etc.), or []`

func (s *Service) score(ctx context.Context, c ai.Client, app Applicant, srch Search, l Listing) (scoreReply, error) {
	var prefs strings.Builder
	fmt.Fprintf(&prefs, "Searching for: %s\n", strings.Join(srch.Keywords, ", "))
	if len(srch.WorkModes) > 0 {
		fmt.Fprintf(&prefs, "Work modes wanted: %s\n", strings.Join(srch.WorkModes, ", "))
	}
	if len(srch.Locations) > 0 {
		fmt.Fprintf(&prefs, "Locations: %s\n", strings.Join(srch.Locations, ", "))
	}
	if len(srch.Seniority) > 0 {
		fmt.Fprintf(&prefs, "Seniority wanted: %s\n", strings.Join(srch.Seniority, ", "))
	}
	if len(srch.JobTypes) > 0 {
		fmt.Fprintf(&prefs, "Job types: %s\n", strings.ReplaceAll(strings.Join(srch.JobTypes, ", "), "_", " "))
	}
	if srch.MinSalary > 0 {
		fmt.Fprintf(&prefs, "Minimum salary: %d per month\n", srch.MinSalary)
	}
	if srch.Notes != "" {
		fmt.Fprintf(&prefs, "Notes from the candidate: %s\n", srch.Notes)
	}

	user := fmt.Sprintf("<candidate>\n%s</candidate>\n\n<preferences>\n%s</preferences>\n\n<job>\nPortal: %s\nTitle: %s\nCompany: %s\nLocation: %s\nWork mode: %s\nType: %s\nSalary: %s\n\n%s\n</job>",
		app.Text, prefs.String(), PortalLabel[l.Portal], l.Title, l.Company, orUnknown(l.Location), orUnknown(l.WorkMode),
		orUnknown(strings.ReplaceAll(l.JobType, "_", " ")), orUnknown(l.Salary), Clip(orUnknown(l.Description), 9000))

	var r scoreReply
	if err := ai.CompleteJSON(ctx, c, scoreSystem, user, &r); err != nil {
		return r, err
	}
	r.Score = max(0, min(100, r.Score))
	return r, nil
}

func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(not stated)"
	}
	return s
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
