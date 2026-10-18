package jobs

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

var (
	parens     = regexp.MustCompile(`\([^)]*\)|\[[^\]]*\]`)
	nonWord    = regexp.MustCompile(`[^\p{L}\p{N}]+`)
	titleNoise = regexp.MustCompile(`\b(urgent|hiring|wfh|wfo|remote|hybrid|onsite|on-site|immediate|join|now|we are|needed|dibutuhkan|segera)\b`)
	companyEnd = regexp.MustCompile(`\b(pt|tbk|inc|ltd|llc|corp|co|group|indonesia)\b`)
	spaces     = regexp.MustCompile(`\s+`)
	required   = regexp.MustCompile(`(?i)\s*(\*|\(required\)|\(wajib\)|required)\s*$`)
)

// Fingerprint identifies "the same job" across portals: company and title
// with noise words, brackets and punctuation removed.
func Fingerprint(company, title string) string {
	return normWords(companyEnd.ReplaceAllString(strings.ToLower(company), " ")) + "|" +
		normWords(titleNoise.ReplaceAllString(parens.ReplaceAllString(strings.ToLower(title), " "), " "))
}

func normWords(s string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(nonWord.ReplaceAllString(s, " "), " "))
}

// QuestionKey normalizes a form label so "Years of experience with Go? *"
// and "years of experience with go" share one saved answer.
func QuestionKey(q string) string {
	q = required.ReplaceAllString(strings.TrimSpace(q), "")
	return normWords(strings.ToLower(q))
}

// HTMLText flattens job-description HTML to readable text with line breaks
// at block elements.
func HTMLText(s string) string {
	if !strings.ContainsAny(s, "<&") {
		return strings.TrimSpace(s)
	}
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		return strings.TrimSpace(s)
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			b.WriteString(n.Data)
		case html.ElementNode:
			switch n.Data {
			case "script", "style", "button":
				return
			case "br":
				b.WriteString("\n")
			case "li":
				b.WriteString("\n• ")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "p", "div", "ul", "ol", "h1", "h2", "h3", "h4", "h5", "h6", "section", "tr":
				b.WriteString("\n")
			}
		}
	}
	walk(doc)
	lines := strings.Split(b.String(), "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, l := range lines {
		l = strings.TrimRightFunc(spaces.ReplaceAllString(l, " "), unicode.IsSpace)
		l = strings.TrimLeft(l, " ")
		if l == "" || l == "•" {
			if !blank && len(out) > 0 {
				out = append(out, "")
			}
			blank = true
			continue
		}
		blank = false
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// Clip shortens s to at most n bytes on a rune boundary.
func Clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8Start(s[n]) {
		n--
	}
	return s[:n] + "…"
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

func containsFold(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

// matchesAny reports whether text contains any of the phrases as whole words.
func matchesAny(text string, phrases []string) string {
	t := " " + normWords(strings.ToLower(text)) + " "
	for _, p := range phrases {
		n := normWords(strings.ToLower(p))
		if n != "" && strings.Contains(t, " "+n+" ") {
			return p
		}
	}
	return ""
}
