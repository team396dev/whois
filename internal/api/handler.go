package api

import (
	"encoding/json"
	"net/http"
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

type lookupRequest struct {
	Domains []string `json:"domains"`
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

	// Deduplicate and sanitize
	seen := map[string]bool{}
	var domains []string
	for _, d := range req.Domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		domains = append(domains, d)
	}

	limit := maxDomains()
	if len(domains) > limit {
		domains = domains[:limit]
	}
	if len(domains) == 0 {
		http.Error(w, "no domains provided", http.StatusBadRequest)
		return
	}

	StreamResults(w, r, domains)
}
