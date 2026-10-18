package analytics

import (
	"net/http/httptest"
	"testing"
)

const chromeMac = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"

func TestMaskIP(t *testing.T) {
	cases := map[string]string{
		"203.0.113.77":        "203.0.113.0",
		"2001:db8:abcd:12::1": "2001:db8:abcd::",
		"not an ip":           "",
		" 198.51.100.4 ":      "198.51.100.0",
	}
	for in, want := range cases {
		if got := MaskIP(in); got != want {
			t.Errorf("MaskIP(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsBot(t *testing.T) {
	bots := []string{
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; GPTBot/1.4; +https://openai.com/gptbot)",
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"facebookexternalhit/1.1",
		"curl/8.7.1",
		"",
	}
	for _, ua := range bots {
		if !IsBot(ua) {
			t.Errorf("not recognised as a bot: %q", ua)
		}
	}
	people := []string{chromeMac, "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1"}
	for _, ua := range people {
		if IsBot(ua) {
			t.Errorf("a person flagged as a bot: %q", ua)
		}
	}
}

func TestParseUA(t *testing.T) {
	if d, b, o := ParseUA(chromeMac); d != "desktop" || b != "Chrome" || o != "macOS" {
		t.Errorf("chrome mac: %s %s %s", d, b, o)
	}
	iphone := "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1"
	if d, b, o := ParseUA(iphone); d != "mobile" || b != "Safari" || o != "iOS" {
		t.Errorf("iphone: %s %s %s", d, b, o)
	}
}

func TestFromRequestUsesCloudflare(t *testing.T) {
	r := httptest.NewRequest("GET", "https://ariffadlysiregar.my.id/api/cv", nil)
	r.RemoteAddr = "182.253.10.44:0" // set by the CloudflareIP middleware
	r.Header.Set("User-Agent", chromeMac)
	r.Header.Set("CF-IPCountry", "ID")
	r.Header.Set("CF-IPCity", "South%20Jakarta")
	r.Header.Set("CF-Region", "Jakarta")
	r.Header.Set("Referer", "https://www.linkedin.com/in/someone/")
	v := FromRequest(r, "cv", true, false)
	if v.IP != "182.253.10.0" || v.CountryCode != "ID" || v.City != "South Jakarta" || v.Region != "Jakarta" ||
		v.Referrer != "linkedin.com" || v.Bot || v.Device != "desktop" {
		t.Fatalf("visit %+v", v)
	}
	// Without a trusted proxy, location headers are ignored (anyone can send them).
	if v := FromRequest(r, "cv", false, false); v.CountryCode != "" || v.City != "" {
		t.Fatalf("untrusted headers used: %+v", v)
	}
	// The site linking to itself is not a referrer.
	r.Header.Set("Referer", "https://ariffadlysiregar.my.id/")
	if v := FromRequest(r, "view", true, false); v.Referrer != "" {
		t.Fatalf("own site as referrer: %q", v.Referrer)
	}
}

func TestCountryName(t *testing.T) {
	if CountryName("ID") != "Indonesia" || CountryName("SG") != "Singapore" || CountryName("") != "Unknown" {
		t.Fatal(CountryName("ID"), CountryName("SG"))
	}
}
