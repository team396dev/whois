package httpcheck

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const (
	checkTimeout = 15 * time.Second
	maxBody      = 512 * 1024

	uaChrome    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	uaGooglebot = "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"
	refGoogle   = "https://www.google.com/"
)

var checkClient = &http.Client{
	Timeout: checkTimeout,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return http.ErrUseLastResponse
		}
		return nil
	},
}

// AltInfo holds a single <link rel="alternate"> tag.
type AltInfo struct {
	Href     string `json:"href"`
	HrefLang string `json:"hreflang,omitempty"`
	Relation string `json:"relation"` // "match"|"subfolder"|"subdomain"|"other_site"
}

// CanonInfo holds parsed canonical and alternate data from <head>.
type CanonInfo struct {
	URL      string    `json:"url,omitempty"`
	Relation string    `json:"relation"` // "match"|"subfolder"|"subdomain"|"other_site"|"none"
	Alts     []AltInfo `json:"alts,omitempty"`
}

// Result holds HTTP status codes and content similarity metrics for a domain.
type Result struct {
	DirectCode int       `json:"direct_code"`           // Chrome UA, no referer
	BotCode    int       `json:"bot_code"`              // Googlebot UA
	RefCode    int       `json:"ref_code"`              // Chrome UA + Referer: google.com
	SimBotDir  *int      `json:"sim_bot_dir,omitempty"` // Googlebot vs Direct similarity (0-100)
	SimRefDir  *int      `json:"sim_ref_dir,omitempty"` // Google-ref'd vs Direct similarity (0-100)
	Canon      *CanonInfo `json:"canon,omitempty"`
	Error      string    `json:"http_error,omitempty"`
}

type fetchResp struct {
	code int
	body string
	err  error
}

// Check performs three concurrent HTTP GETs to https://domain and compares page content.
func Check(domain string) Result {
	domain = strings.TrimPrefix(strings.ToLower(domain), "www.")
	url := "https://" + domain

	directCh := make(chan fetchResp, 1)
	botCh := make(chan fetchResp, 1)
	refCh := make(chan fetchResp, 1)

	go func() {
		code, body, err := fetch(url, uaChrome, "")
		directCh <- fetchResp{code, body, err}
	}()
	go func() {
		code, body, err := fetch(url, uaGooglebot, "")
		botCh <- fetchResp{code, body, err}
	}()
	go func() {
		code, body, err := fetch(url, uaChrome, refGoogle)
		refCh <- fetchResp{code, body, err}
	}()

	direct := <-directCh
	bot := <-botCh
	ref := <-refCh

	if direct.err != nil && bot.err != nil && ref.err != nil {
		return Result{Error: trimError(direct.err.Error())}
	}

	r := Result{
		DirectCode: direct.code,
		BotCode:    bot.code,
		RefCode:    ref.code,
	}

	if direct.err == nil && bot.err == nil && is2xx(direct.code) && is2xx(bot.code) {
		v := textSimilarity(direct.body, bot.body)
		r.SimBotDir = &v
	}
	if direct.err == nil && ref.err == nil && is2xx(direct.code) && is2xx(ref.code) {
		v := textSimilarity(direct.body, ref.body)
		r.SimRefDir = &v
	}
	if direct.err == nil && is2xx(direct.code) {
		r.Canon = parseCanonical(direct.body, domain)
	}

	return r
}

func is2xx(code int) bool { return code >= 200 && code < 300 }

func trimError(s string) string {
	if strings.Contains(s, "context deadline exceeded") || strings.Contains(s, "timeout") {
		return "timeout"
	}
	if strings.Contains(s, "no such host") {
		return "DNS error"
	}
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}

func fetch(url, userAgent, referer string) (int, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("User-Agent", userAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := checkClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return resp.StatusCode, "", err
	}
	return resp.StatusCode, string(body), nil
}

var (
	tagRe      = regexp.MustCompile(`<[^>]+>`)
	canonRe    = regexp.MustCompile(`(?i)<link[^>]+rel=["']canonical["'][^>]*>|<link[^>]+href=["']([^"']+)["'][^>]*rel=["']canonical["'][^>]*>`)
	canonHref  = regexp.MustCompile(`(?i)href=["']([^"']+)["']`)
	altRe      = regexp.MustCompile(`(?i)<link[^>]+rel=["']alternate["'][^>]*>`)
	altHrefRe  = regexp.MustCompile(`(?i)href=["']([^"']+)["']`)
	altLangRe  = regexp.MustCompile(`(?i)hreflang=["']([^"']+)["']`)
)

func classifyHref(href, domain string) string {
	u, err := url.Parse(href)
	if err != nil {
		return "other_site"
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if host == domain {
		p := u.Path
		if p == "" || p == "/" {
			return "match"
		}
		return "subfolder"
	}
	if strings.HasSuffix(host, "."+domain) {
		return "subdomain"
	}
	return "other_site"
}

func parseCanonical(body, domain string) *CanonInfo {
	// extract <head> only to avoid false matches in body text
	head := body
	if i := strings.Index(strings.ToLower(body), "</head>"); i > 0 {
		head = body[:i]
	}

	info := &CanonInfo{Relation: "none"}

	if m := canonRe.FindString(head); m != "" {
		if h := canonHref.FindStringSubmatch(m); len(h) > 1 {
			info.URL = h[1]
			info.Relation = classifyHref(h[1], domain)
		}
	}

	for _, tag := range altRe.FindAllString(head, -1) {
		hm := altHrefRe.FindStringSubmatch(tag)
		if len(hm) < 2 {
			continue
		}
		ai := AltInfo{
			Href:     hm[1],
			Relation: classifyHref(hm[1], domain),
		}
		if lm := altLangRe.FindStringSubmatch(tag); len(lm) > 1 {
			ai.HrefLang = lm[1]
		}
		info.Alts = append(info.Alts, ai)
	}

	if info.URL == "" && len(info.Alts) == 0 {
		return nil
	}
	return info
}

func stripTags(html string) string {
	return tagRe.ReplaceAllString(html, " ")
}

func wordSet(text string) map[string]bool {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	set := make(map[string]bool, len(words))
	for _, w := range words {
		if len(w) > 2 {
			set[w] = true
		}
	}
	return set
}

// textSimilarity computes Jaccard word-set similarity (0–100) between two HTML pages.
func textSimilarity(a, b string) int {
	aSet := wordSet(stripTags(a))
	bSet := wordSet(stripTags(b))

	if len(aSet) == 0 && len(bSet) == 0 {
		return 100
	}
	if len(aSet) == 0 || len(bSet) == 0 {
		return 0
	}

	intersection := 0
	for w := range aSet {
		if bSet[w] {
			intersection++
		}
	}
	union := len(aSet) + len(bSet) - intersection
	return int(float64(intersection) / float64(union) * 100)
}
