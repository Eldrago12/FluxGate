package api

import (
	"net/http"
	"time"

	"github.com/Eldrago12/FluxGate/internal/limiter"
)

type Handler struct {
	distributedLimiter *limiter.DistributedLimiter
	dynamicLimiter     *limiter.DynamicLimiter
	apiToLimit         string
}

func NewHandler(distributedLimiter *limiter.DistributedLimiter, dynamicLimiter *limiter.DynamicLimiter, apiToLimit string) *Handler {
	return &Handler{
		distributedLimiter: distributedLimiter,
		dynamicLimiter:     dynamicLimiter,
		apiToLimit:         apiToLimit,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Use both limiters
	allowed := h.distributedLimiter.Allow(h.apiToLimit) && h.dynamicLimiter.Allow()

	if allowed {
		// Forward the request to the rate-limited API
		client := &http.Client{}
		req, err := http.NewRequest(r.Method, h.apiToLimit, r.Body)
		if err != nil {
			http.Error(w, "Error creating request", http.StatusInternalServerError)
			h.dynamicLimiter.RecordMetrics(time.Since(start), true)
			return
		}

		// Copy headers from original request
		for name, values := range r.Header {
			for _, value := range values {
				req.Header.Add(name, value)
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "Error forwarding request", http.StatusInternalServerError)
			h.dynamicLimiter.RecordMetrics(time.Since(start), true)
			return
		}
		defer resp.Body.Close()

		// Copy the response from the rate-limited API back to the client
		for name, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		w.WriteHeader(resp.StatusCode)
		http.MaxBytesReader(w, resp.Body, 1048576) // Limit response to 1MB
		_, _ = w.Write([]byte("Request allowed and forwarded"))
		h.dynamicLimiter.RecordMetrics(time.Since(start), false)
	} else {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("Rate limit exceeded"))
		h.dynamicLimiter.RecordMetrics(time.Since(start), true)
	}
}
