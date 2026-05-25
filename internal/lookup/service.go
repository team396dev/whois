package lookup

import (
	"context"
	"strings"
	"sync"
	"time"

	"whois-parser/internal/contentcheck"
	"whois-parser/internal/httpcheck"
	"whois-parser/internal/rdap"
	"whois-parser/internal/whois"

	"golang.org/x/time/rate"
)

type Result struct {
	Domain    string               `json:"domain"`
	Registrar string               `json:"registrar,omitempty"`
	Source    string               `json:"source"` // "whois" | "rdap" | "error"
	Error     string               `json:"error,omitempty"`
	HTTP      *httpcheck.Result    `json:"http,omitempty"`
	Content   *contentcheck.Result `json:"content,omitempty"`
}

// per-server rate limiters to avoid hammering WHOIS servers.
var (
	limitersMu sync.Mutex
	limiters   = map[string]*rate.Limiter{}
)

func getLimiter(server string, interval time.Duration) *rate.Limiter {
	if interval == 0 {
		interval = 500 * time.Millisecond
	}
	limitersMu.Lock()
	defer limitersMu.Unlock()
	if l, ok := limiters[server]; ok {
		return l
	}
	l := rate.NewLimiter(rate.Every(interval), 3)
	limiters[server] = l
	return l
}

func extractTLD(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-1]
}

// Lookup resolves WHOIS/RDAP, runs HTTP checks, and optionally checks content terms — all concurrently.
func Lookup(domain string, terms []string) Result {
	domain = strings.ToLower(strings.TrimSpace(domain))
	tld := extractTLD(domain)
	if tld == "" {
		return Result{Domain: domain, Source: "error", Error: "invalid domain"}
	}

	httpCh := make(chan httpcheck.Result, 1)
	go func() { httpCh <- httpcheck.Check(domain) }()

	var contentCh chan contentcheck.Result
	if len(terms) > 0 {
		contentCh = make(chan contentcheck.Result, 1)
		go func() { contentCh <- contentcheck.Check(domain, terms) }()
	}

	var registrar, source string

	cfg, ok := whois.TLDConfigs[tld]
	if ok {
		limiter := getLimiter(cfg.Server, cfg.RateLimit)
		_ = limiter.Wait(context.Background())

		raw, err := whois.Query(domain, cfg)
		if err == nil {
			reg := whois.ExtractRegistrar(raw, cfg.RegistrarField)
			if reg != "" {
				registrar, source = reg, "whois"
			}
		}
	} else {
		raw, err := whois.QueryIANA(domain)
		if err == nil {
			reg := whois.ExtractRegistrar(raw, []string{"Registrar:", "registrar:", "Registrar Name:"})
			if reg != "" {
				registrar, source = reg, "whois"
			}
		}
	}

	if registrar == "" {
		reg, err := rdap.Query(domain)
		if err == nil && reg != "" {
			registrar, source = reg, "rdap"
		}
	}

	httpResult := <-httpCh

	res := Result{Domain: domain, Source: source, HTTP: &httpResult}
	if registrar != "" {
		res.Registrar = registrar
	} else {
		res.Source = "error"
		res.Error = "registrar not found"
	}

	if contentCh != nil {
		cr := <-contentCh
		res.Content = &cr
	}

	return res
}
