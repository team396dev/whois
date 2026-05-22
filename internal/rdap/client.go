package rdap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	rdapTimeout      = 10 * time.Second
	bootstrapURL     = "https://data.iana.org/rdap/dns.json"
	bootstrapMaxAge  = 24 * time.Hour
)

var httpClient = &http.Client{
	Timeout: rdapTimeout,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	},
}

// bootstrapCache stores TLD→RDAP base URL mappings from IANA.
var (
	bsMu      sync.RWMutex
	bsMap     map[string]string
	bsFetched time.Time
)

type bootstrapJSON struct {
	Services [][][]string `json:"services"`
}

func getBootstrap() map[string]string {
	bsMu.RLock()
	if bsMap != nil && time.Since(bsFetched) < bootstrapMaxAge {
		m := bsMap
		bsMu.RUnlock()
		return m
	}
	bsMu.RUnlock()

	bsMu.Lock()
	defer bsMu.Unlock()
	// Double-check after acquiring write lock
	if bsMap != nil && time.Since(bsFetched) < bootstrapMaxAge {
		return bsMap
	}

	ctx, cancel := context.WithTimeout(context.Background(), rdapTimeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, bootstrapURL, nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return bsMap // return stale on error
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return bsMap
	}

	var bs bootstrapJSON
	if err := json.Unmarshal(body, &bs); err != nil {
		return bsMap
	}

	m := make(map[string]string, 512)
	for _, svc := range bs.Services {
		if len(svc) != 2 || len(svc[1]) == 0 {
			continue
		}
		base := strings.TrimSuffix(svc[1][0], "/")
		for _, tld := range svc[0] {
			m[strings.ToLower(tld)] = base
		}
	}

	bsMap = m
	bsFetched = time.Now()
	return m
}

// Query fetches the registrar name for domain via RDAP.
// It uses the IANA bootstrap to find the authoritative RDAP server.
func Query(domain string) (string, error) {
	tld := extractTLD(domain)
	if tld == "" {
		return "", fmt.Errorf("cannot extract TLD from %q", domain)
	}

	bs := getBootstrap()
	baseURL, ok := bs[tld]
	if !ok {
		return "", fmt.Errorf("no RDAP server known for .%s", tld)
	}

	url := baseURL + "/domain/" + domain
	return queryURL(url)
}

func extractTLD(domain string) string {
	parts := strings.Split(strings.ToLower(domain), ".")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-1]
}

func queryURL(url string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), rdapTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/rdap+json, application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("rdap request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("domain not found (404)")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("rdap status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", fmt.Errorf("rdap read: %w", err)
	}

	var result rdapResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("rdap parse: %w", err)
	}

	return extractRegistrar(result)
}

type rdapResponse struct {
	Entities []rdapEntity `json:"entities"`
}

type rdapEntity struct {
	Roles      []string    `json:"roles"`
	VCardArray interface{} `json:"vcardArray"`
	Entities   []rdapEntity `json:"entities"`
}

func extractRegistrar(r rdapResponse) (string, error) {
	// Search top-level entities first
	for _, entity := range r.Entities {
		if hasRole(entity.Roles, "registrar") {
			name := extractFN(entity.VCardArray)
			if name != "" {
				return name, nil
			}
		}
	}
	// Some registries nest registrar inside another entity
	for _, entity := range r.Entities {
		for _, child := range entity.Entities {
			if hasRole(child.Roles, "registrar") {
				name := extractFN(child.VCardArray)
				if name != "" {
					return name, nil
				}
			}
		}
	}
	return "", fmt.Errorf("registrar not found in RDAP response")
}

func hasRole(roles []string, target string) bool {
	for _, r := range roles {
		if r == target {
			return true
		}
	}
	return false
}

// extractFN navigates vcardArray[1] looking for a ["fn", ..., ..., value] entry.
func extractFN(vcardArray interface{}) string {
	outer, ok := vcardArray.([]interface{})
	if !ok || len(outer) < 2 {
		return ""
	}
	props, ok := outer[1].([]interface{})
	if !ok {
		return ""
	}
	for _, prop := range props {
		arr, ok := prop.([]interface{})
		if !ok || len(arr) < 4 {
			continue
		}
		key, ok := arr[0].(string)
		if !ok || key != "fn" {
			continue
		}
		if val, ok := arr[3].(string); ok {
			return val
		}
	}
	return ""
}
