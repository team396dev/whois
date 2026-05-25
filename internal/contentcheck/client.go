package contentcheck

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	timeout = 15 * time.Second
	maxBody = 512 * 1024

	uaDirect    = "Go-http-client/1.1"
	uaGooglebot = "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"
	uaChrome    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	refGoogle   = "https://www.google.com/"
)

var client = &http.Client{
	Timeout: timeout,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return http.ErrUseLastResponse
		}
		return nil
	},
}

var tagRe = regexp.MustCompile(`<[^>]+>`)

// TermCount holds match count for a single term.
type TermCount struct {
	Term  string `json:"term"`
	Count int    `json:"count"`
}

// RequestResult holds term counts for one request type.
type RequestResult struct {
	Total  int         `json:"total"`
	ByTerm []TermCount `json:"by_term"`
	Error  string      `json:"error,omitempty"`
}

// Result holds content check results for a domain across three request types.
type Result struct {
	Domain string        `json:"domain"`
	Direct RequestResult `json:"direct"`
	Bot    RequestResult `json:"bot"`
	Ref    RequestResult `json:"ref"`
}

// Check fetches a domain three ways and counts term occurrences in visible text.
func Check(domain string, terms []string) Result {
	url := "https://" + domain

	type fetchOut struct {
		body string
		err  string
	}

	directCh := make(chan fetchOut, 1)
	botCh := make(chan fetchOut, 1)
	refCh := make(chan fetchOut, 1)

	go func() { body, err := fetchBody(url, uaDirect, ""); directCh <- fetchOut{body, err} }()
	go func() { body, err := fetchBody(url, uaGooglebot, ""); botCh <- fetchOut{body, err} }()
	go func() { body, err := fetchBody(url, uaChrome, refGoogle); refCh <- fetchOut{body, err} }()

	d := <-directCh
	b := <-botCh
	rf := <-refCh

	return Result{
		Domain: domain,
		Direct: countTerms(d.body, d.err, terms),
		Bot:    countTerms(b.body, b.err, terms),
		Ref:    countTerms(rf.body, rf.err, terms),
	}
}

func fetchBody(rawURL, userAgent, referer string) (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err.Error()
	}
	req.Header.Set("User-Agent", userAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := client.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") || strings.Contains(err.Error(), "timeout") {
			return "", "timeout"
		}
		if strings.Contains(err.Error(), "no such host") {
			return "", "DNS error"
		}
		return "", err.Error()
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return "", err.Error()
	}
	return string(raw), ""
}

func countTerms(body, errMsg string, terms []string) RequestResult {
	if errMsg != "" {
		return RequestResult{Error: errMsg}
	}
	text := strings.ToLower(tagRe.ReplaceAllString(body, " "))
	var counts []TermCount
	total := 0
	for _, t := range terms {
		tl := strings.ToLower(t)
		n := strings.Count(text, tl)
		counts = append(counts, TermCount{Term: t, Count: n})
		total += n
	}
	return RequestResult{Total: total, ByTerm: counts}
}
