package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

func maxDomains() int {
	if v := os.Getenv("MAX_DOMAINS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 500
}

// normalizeDomain strips URL scheme, credentials, path, port, and www prefix.
// Accepts plain domains (example.com) and full URLs (https://www.example.com/path).
func normalizeDomain(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	if d == "" {
		return ""
	}
	if !strings.Contains(d, "://") {
		d = "https://" + d
	}
	u, err := url.Parse(d)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	host := u.Hostname()
	host = strings.TrimPrefix(host, "www.")
	return host
}

type lookupRequest struct {
	Domains []string `json:"domains"`
	Terms   []string `json:"terms,omitempty"`
}

// LookupHandler handles POST /api/lookup and streams results as SSE.
func LookupHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req lookupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	seen := map[string]bool{}
	var domains []string
	for _, d := range req.Domains {
		d = normalizeDomain(d)
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		domains = append(domains, d)
	}

	var terms []string
	for _, t := range req.Terms {
		if t = strings.TrimSpace(t); t != "" {
			terms = append(terms, t)
		}
	}

	limit := maxDomains()
	if len(domains) > limit {
		domains = domains[:limit]
	}
	if len(domains) == 0 {
		http.Error(w, "no domains provided", http.StatusBadRequest)
		return
	}

	StreamResults(w, r, domains, terms)
}
