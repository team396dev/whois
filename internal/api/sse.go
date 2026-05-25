package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"whois-parser/internal/lookup"
)

type doneEvent struct {
	Type  string         `json:"type"`
	Stats map[string]int `json:"stats"`
	Total int            `json:"total"`
}

// StreamResults runs the batch lookup and pushes each result as an SSE event.
func StreamResults(w http.ResponseWriter, r *http.Request, domains []string, terms []string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	resultCh := make(chan lookup.Result, len(domains))
	go lookup.RunBatch(domains, terms, resultCh)

	stats := map[string]int{}
	total := 0

	for result := range resultCh {
		select {
		case <-r.Context().Done():
			return
		default:
		}

		data, err := json.Marshal(result)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		if result.Registrar != "" {
			stats[result.Registrar]++
		}
		total++
	}

	done := doneEvent{Type: "done", Stats: stats, Total: total}
	data, _ := json.Marshal(done)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}
