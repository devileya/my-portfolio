package jobs

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/devileya/my-portfolio/internal/jobs/browser"
)

func TestFingerprintMatchesAcrossPortals(t *testing.T) {
	a := Fingerprint("PT Kredivo Group Indonesia", "Backend Engineer - Golang (SDE II) [Remote]")
	b := Fingerprint("Kredivo Group", "Backend Engineer – Golang")
	if a != b {
		t.Fatalf("fingerprints differ:\n%q\n%q", a, b)
	}
	if Fingerprint("Acme", "Frontend Engineer") == Fingerprint("Acme", "Backend Engineer") {
		t.Fatal("different titles share a fingerprint")
	}
}

func TestQuestionKey(t *testing.T) {
	for _, q := range []string{"Years of experience with Go? *", "years of experience with go", "Years of Experience with Go (required)"} {
		if got := QuestionKey(q); got != "years of experience with go" {
			t.Errorf("QuestionKey(%q) = %q", q, got)
		}
	}
}

func TestHTMLText(t *testing.T) {
	got := HTMLText("<p>We build <strong>things</strong>.</p><ul><li>Go</li><li>Postgres</li></ul><script>x()</script>")
	want := "We build things.\n\n• Go\n• Postgres"
	if got != want {
		t.Fatalf("HTMLText:\n%q\nwant\n%q", got, want)
	}
}

func TestMatchesAnyIsWholeWord(t *testing.T) {
	if matchesAny("Senior Golang Engineer", []string{"go"}) != "" {
		t.Error("“go” should not match inside “Golang”")
	}
	if matchesAny("PHP Developer (Laravel)", []string{"php"}) != "php" {
		t.Error("“php” should match")
	}
}

func TestNextRunUsesTimezone(t *testing.T) {
	after := time.Date(2026, 10, 18, 2, 30, 0, 0, time.UTC) // 09:30 in Jakarta
	next, err := NextRun("0 9,13,17 * * *", "Asia/Jakarta", after)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 10, 18, 6, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Fatalf("next = %s, want %s", next.UTC(), want)
	}
	if _, err := NextRun("every day", "UTC", after); err == nil {
		t.Fatal("expected an error for a bad schedule")
	}
}

func TestFitsOptions(t *testing.T) {
	f := browser.Field{Kind: "select", Options: []string{"Yes", "No"}}
	if !fits(f, "yes") || fits(f, "Maybe") {
		t.Fatal("option matching is wrong")
	}
	if !fits(browser.Field{Kind: "text"}, "anything") {
		t.Fatal("free text always fits")
	}
}

func TestPickButtonPrefersSubmit(t *testing.T) {
	b, ok := pickButton([]browser.Button{{Kind: "other"}, {Kind: "next", Text: "Next"}, {Kind: "submit", Text: "Submit application"}})
	if !ok || b.Kind != "submit" {
		t.Fatalf("picked %+v", b)
	}
	if _, ok := pickButton([]browser.Button{{Kind: "other"}}); ok {
		t.Fatal("no usable button should be reported")
	}
}

func TestSuccessText(t *testing.T) {
	for _, s := range []string{"Your application was sent to Kredivo", "Lamaran kamu berhasil dikirim!", "Thank you for applying"} {
		if !successText.MatchString(s) {
			t.Errorf("not recognised as success: %q", s)
		}
	}
	if successText.MatchString("Submit application") {
		t.Error("a button label is not success")
	}
}

func TestKalibrrListing(t *testing.T) {
	j := kalibrrJob{ID: 7, Name: "Golang Developer", Slug: "golang-developer-21", Tenure: "Full time", IsWFH: true}
	j.Company.Code, j.Company.Name = "mobius-digital", "Mobius Digital"
	l := j.listing()
	if l.URL != "https://www.kalibrr.com/c/mobius-digital/jobs/7/golang-developer-21" || l.WorkMode != "remote" || l.JobType != "full_time" {
		t.Fatalf("unexpected listing %+v", l)
	}
	if l.EasyApply == nil || !*l.EasyApply {
		t.Fatal("no redirect URL means apply on Kalibrr")
	}
}

func TestLinkedInCardParsing(t *testing.T) {
	card := `<li><div class="base-card" data-entity-urn="urn:li:jobPosting:4440204415">
		<h3 class="base-search-card__title">
			Backend Engineer - Golang (SDE II)
		</h3>
		<h4 class="base-search-card__subtitle"><a href="x">
			Kredivo Group
		</a></h4>
		<span class="job-search-card__location">
			Jakarta Metropolitan Area
		</span>
		<time class="job-search-card__listdate" datetime="2026-10-09">1 week ago</time></div></li>`
	m := liCard.FindAllStringSubmatch(card, -1)
	if len(m) != 1 || m[0][1] != "4440204415" {
		t.Fatalf("card not found: %v", m)
	}
	if got := cleanHTML(first(liTitle, m[0][0])); got != "Backend Engineer - Golang (SDE II)" {
		t.Errorf("title %q", got)
	}
	if got := cleanHTML(first(liCompany, m[0][0])); got != "Kredivo Group" {
		t.Errorf("company %q", got)
	}
	if got := first(liDate, m[0][0]); got != "2026-10-09" {
		t.Errorf("date %q", got)
	}
	if !strings.Contains(cleanHTML(first(liLocation, m[0][0])), "Jakarta") {
		t.Error("location missing")
	}
}

func TestParseSalary(t *testing.T) {
	cases := []struct {
		in               string
		min, max         int64
		currency, period string
	}{
		{"Rp 8.000.000 - Rp 12.000.000 per bulan", 8_000_000, 12_000_000, "IDR", "month"},
		{"IDR 10.8M – 11.8M / month", 10_800_000, 11_800_000, "IDR", "month"},
		{"$80,000.00/yr - $100,000.00/yr", 80_000, 100_000, "USD", "year"},
		{"Rp 15 jt", 15_000_000, 15_000_000, "IDR", ""},
		{"IDR 15,000,000.00/month - IDR 20,000,000.00/month", 15_000_000, 20_000_000, "IDR", "month"},
	}
	for _, c := range cases {
		min, max, cur, period, ok := ParseSalary(c.in)
		if !ok || min != c.min || max != c.max || cur != c.currency || period != c.period {
			t.Errorf("ParseSalary(%q) = %d %d %q %q %v", c.in, min, max, cur, period, ok)
		}
	}
	if _, _, _, _, ok := ParseSalary("Competitive"); ok {
		t.Error("no figures means no range")
	}
}

func TestSalaryAsk(t *testing.T) {
	lo, hi := int64(10_000_000), int64(12_500_000)
	l := Listing{SalaryMin: &lo, SalaryMax: &hi, SalaryCurrency: "IDR", SalaryPeriod: "month"}
	if got := SalaryAsk(l, ""); !strings.HasPrefix(got, "IDR 12,500,000 per month") {
		t.Errorf("top: %q", got)
	}
	if got := SalaryAsk(l, "middle"); !strings.HasPrefix(got, "IDR 11,300,000 per month") {
		t.Errorf("middle: %q", got)
	}
	if SalaryAsk(l, "off") != "" || SalaryAsk(Listing{}, "") != "" {
		t.Error("off, or no range, gives no per-job salary")
	}
}

func TestSalaryQuestions(t *testing.T) {
	if !isSalaryQuestion("Berapa gaji yang Anda harapkan?") || !isSalaryQuestion("Expected salary (IDR)") {
		t.Error("salary questions not recognised")
	}
	if !isCurrentSalary("What is your current salary?") || isCurrentSalary("Expected salary") {
		t.Error("current vs expected salary mixed up")
	}
	if isSalaryQuestion("Years of experience with Go") {
		t.Error("not a salary question")
	}
}

func TestJobContextSalary(t *testing.T) {
	hi := int64(20_000_000)
	s := &Service{}
	app := Applicant{Facts: map[string]string{"salaryFromRange": "top"}}
	jc := s.jobContext(context.Background(), Listing{SalaryMax: &hi, SalaryCurrency: "IDR", SalaryPeriod: "month"}, app)
	if jc.salaryAsk == "" || !strings.Contains(jc.text, "expectedSalaryForThisJob: IDR 20,000,000 per month") {
		t.Fatalf("job context:\n%s", jc.text)
	}
	if jc := s.jobContext(context.Background(), Listing{}, app); jc.salaryAsk != "" {
		t.Fatalf("no range and no search minimum must leave salary to the user, got %q", jc.salaryAsk)
	}
}
