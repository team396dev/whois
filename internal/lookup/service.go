package lookup

import (
	"context"
	"strings"
	"sync"
	"time"

	"whois-parser/internal/rdap"
	"whois-parser/internal/whois"

	"golang.org/x/time/rate"
)

type Result struct {
	Domain    string `json:"domain"`
	Registrar string `json:"registrar,omitempty"`
	Source    string `json:"source"` // "whois" | "rdap" | "error"
	Error     string `json:"error,omitempty"`
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

// Lookup resolves the registrar for a domain via WHOIS, falling back to RDAP.
func Lookup(domain string) Result {
	domain = strings.ToLower(strings.TrimSpace(domain))
	tld := extractTLD(domain)
	if tld == "" {
		return Result{Domain: domain, Source: "error", Error: "invalid domain"}
	}

	cfg, ok := whois.TLDConfigs[tld]
	if ok {
		limiter := getLimiter(cfg.Server, cfg.RateLimit)
		_ = limiter.Wait(context.Background())

		raw, err := whois.Query(domain, cfg)
		if err == nil {
			reg := whois.ExtractRegistrar(raw, cfg.RegistrarField)
			if reg != "" {
				return Result{Domain: domain, Registrar: reg, Source: "whois"}
			}
		}
	} else {
		// Unknown TLD — try IANA two-step discovery
		raw, err := whois.QueryIANA(domain)
		if err == nil {
			reg := whois.ExtractRegistrar(raw, []string{"Registrar:", "registrar:", "Registrar Name:"})
			if reg != "" {
				return Result{Domain: domain, Registrar: reg, Source: "whois"}
			}
		}
	}

	// RDAP fallback
	reg, err := rdap.Query(domain)
	if err == nil && reg != "" {
		return Result{Domain: domain, Registrar: reg, Source: "rdap"}
	}

	return Result{Domain: domain, Source: "error", Error: "registrar not found"}
}
