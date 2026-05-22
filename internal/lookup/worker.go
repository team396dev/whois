package lookup

import (
	"os"
	"runtime"
	"strconv"
	"sync"
)

func numWorkers() int {
	if v := os.Getenv("WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	n := runtime.NumCPU()
	if n < 4 {
		n = 4 // minimum concurrency for I/O-bound work
	}
	return n
}

// RunBatch fans out lookups across a worker pool, streaming results to resultCh.
// resultCh is closed when all domains have been processed.
func RunBatch(domains []string, resultCh chan<- Result) {
	workers := numWorkers()
	jobs := make(chan string, len(domains))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for domain := range jobs {
				resultCh <- Lookup(domain)
			}
		}()
	}

	for _, d := range domains {
		jobs <- d
	}
	close(jobs)

	wg.Wait()
	close(resultCh)
}
