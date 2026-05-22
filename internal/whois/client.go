package whois

import (
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const (
	dialTimeout  = 10 * time.Second
	readDeadline = 15 * time.Second
)

// Query performs a WHOIS TCP lookup on port 43.
func Query(domain string, cfg TLDConfig) (string, error) {
	conn, err := net.DialTimeout("tcp", cfg.Server, dialTimeout)
	if err != nil {
		return "", fmt.Errorf("dial %s: %w", cfg.Server, err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(readDeadline)); err != nil {
		return "", err
	}

	query := domain + "\r\n"
	if cfg.QueryPrefix != "" {
		query = cfg.QueryPrefix + " " + query
	}
	if _, err := fmt.Fprint(conn, query); err != nil {
		return "", fmt.Errorf("write query: %w", err)
	}

	buf, err := io.ReadAll(conn)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	return string(buf), nil
}

// QueryIANA performs a two-step WHOIS lookup via whois.iana.org to discover
// the authoritative WHOIS server, then queries it.
func QueryIANA(domain string) (string, error) {
	ianaCfg := TLDConfig{Server: "whois.iana.org:43", RegistrarField: []string{"refer:"}}
	raw, err := Query(domain, ianaCfg)
	if err != nil {
		return "", fmt.Errorf("IANA lookup: %w", err)
	}

	referServer := ""
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "refer:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				referServer = strings.TrimSpace(parts[1]) + ":43"
				break
			}
		}
	}
	if referServer == "" {
		return "", fmt.Errorf("no refer server found in IANA response")
	}

	return Query(domain, TLDConfig{Server: referServer, RegistrarField: []string{"Registrar:", "registrar:"}})
}
