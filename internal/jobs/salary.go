package jobs

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var (
	salaryNumber = regexp.MustCompile(`(?i)(\d[\d.,]*)\s*(k|m|jt|juta|rb|ribu)?\b`)
	salaryQ      = regexp.MustCompile(`(?i)salary|gaji|compensation|remuneration|pay (expectation|range)|\bctc\b|take.?home|\bthp\b|upah|penghasilan|income`)
	currentQ     = regexp.MustCompile(`(?i)current|present|last|previous|saat ini|terakhir|sekarang|sebelumnya`)
	perMonth     = regexp.MustCompile(`month|bulan|/mo\b|monthly`)
	perYear      = regexp.MustCompile(`year|/yr|tahun|annual|annum|p\.a\.`)
	perHour      = regexp.MustCompile(`hour|/hr|jam`)
)

// fillSalary derives the numeric range from the salary text when an adapter
// didn't set it directly.
func (l *Listing) fillSalary() {
	if l.SalaryMin != nil || l.SalaryMax != nil || strings.TrimSpace(l.Salary) == "" {
		return
	}
	min, max, cur, period, ok := ParseSalary(l.Salary)
	if !ok {
		return
	}
	l.SalaryMin, l.SalaryMax, l.SalaryCurrency = &min, &max, cur
	if l.SalaryPeriod == "" {
		l.SalaryPeriod = period
	}
}

// ParseSalary reads ranges as portals write them: "Rp 8.000.000 - Rp
// 12.000.000 per bulan", "IDR 10.8M – 11.8M / month", "$80,000/yr - $100,000/yr",
// "Rp 15 jt". A single figure gives min = max.
func ParseSalary(s string) (min, max int64, currency, period string, ok bool) {
	currency = salaryCurrency(s)
	period = salaryPeriod(s)
	var nums []int64
	for _, m := range salaryNumber.FindAllStringSubmatch(s, -1) {
		v, good := parseAmount(m[1], strings.ToLower(m[2]))
		// Without a currency or a suffix, small numbers are not money ("2-4 years").
		if !good || (v < 1000 && m[2] == "" && currency == "") {
			continue
		}
		nums = append(nums, v)
	}
	if len(nums) == 0 {
		return 0, 0, "", "", false
	}
	min, max = nums[0], nums[0]
	for _, n := range nums[1:] {
		min, max = minI(min, n), maxI(max, n)
	}
	return min, max, currency, period, true
}

func parseAmount(num, suffix string) (int64, bool) {
	// The last separator is a decimal point when 1-2 digits follow it;
	// every other separator groups thousands.
	last := strings.LastIndexAny(num, ".,")
	intPart, frac := num, ""
	if last >= 0 && len(num)-last-1 <= 2 && len(num)-last-1 > 0 {
		intPart, frac = num[:last], num[last+1:]
	}
	intPart = strings.NewReplacer(".", "", ",", "").Replace(intPart)
	v, err := strconv.ParseFloat(intPart+"."+frac+"0", 64)
	if err != nil {
		return 0, false
	}
	switch suffix {
	case "k", "rb", "ribu":
		v *= 1e3
	case "m", "jt", "juta":
		v *= 1e6
	}
	return int64(math.Round(v)), v > 0
}

func salaryCurrency(s string) string {
	l := strings.ToLower(s)
	switch {
	case strings.Contains(l, "idr") || strings.Contains(l, "rp"):
		return "IDR"
	case strings.Contains(l, "sgd") || strings.Contains(l, "s$"):
		return "SGD"
	case strings.Contains(l, "myr") || strings.Contains(l, "rm "):
		return "MYR"
	case strings.Contains(l, "eur") || strings.Contains(l, "€"):
		return "EUR"
	case strings.Contains(l, "usd") || strings.Contains(l, "$"):
		return "USD"
	}
	return ""
}

func salaryPeriod(s string) string {
	l := strings.ToLower(s)
	switch {
	case perMonth.MatchString(l):
		return "month"
	case perYear.MatchString(l):
		return "year"
	case perHour.MatchString(l):
		return "hour"
	}
	return ""
}

func normPeriod(s string) string {
	if p := salaryPeriod(s); p != "" {
		return p
	}
	return strings.ToLower(strings.TrimSpace(s))
}

// SalaryAsk is what to answer a "what's your expected salary" field with for
// this job: a figure from the posting's range when it has one, chosen by the
// salaryFromRange fact ("top" by default, "middle" or "bottom"). It returns
// "" when the posting shows no range or the user turned this off ("off").
func SalaryAsk(l Listing, pick string) string {
	if l.SalaryMax == nil || *l.SalaryMax <= 0 || pick == "off" {
		return ""
	}
	min := *l.SalaryMax
	if l.SalaryMin != nil && *l.SalaryMin > 0 {
		min = *l.SalaryMin
	}
	max := *l.SalaryMax
	v := max
	switch pick {
	case "middle":
		v = roundSalary((min + max) / 2)
	case "bottom":
		v = min
	}
	period := l.SalaryPeriod
	if period == "" {
		period = "period not stated"
	}
	rng := formatMoney(min)
	if min != max {
		rng += " – " + formatMoney(max)
	}
	return strings.TrimSpace(fmt.Sprintf("%s %s per %s (the posting offers %s %s)", l.SalaryCurrency, formatMoney(v), period, l.SalaryCurrency, rng))
}

// roundSalary keeps three significant digits: 11,250,000 → 11,300,000.
func roundSalary(v int64) int64 {
	if v < 1000 {
		return v
	}
	step := int64(math.Pow(10, math.Floor(math.Log10(float64(v)))-2))
	return (v + step/2) / step * step
}

func formatMoney(v int64) string {
	s := strconv.FormatInt(v, 10)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// isSalaryQuestion and isCurrentSalary classify a form label.
func isSalaryQuestion(label string) bool { return salaryQ.MatchString(label) }
func isCurrentSalary(label string) bool {
	return isSalaryQuestion(label) && currentQ.MatchString(label)
}

func minI(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxI(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
