package ethereum

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

type metrics struct {
	requests         [6]atomic.Uint64
	durationNS       atomic.Uint64
	admissionErrors  atomic.Uint64
	completionErrors atomic.Uint64
}
type observedWriter struct {
	http.ResponseWriter
	status int
}

func (w *observedWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Metrics exposes only process aggregates; restrict this route at ingress before production.
func (h *Handler) Metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintln(w, "# TYPE infra_rpc_requests_total counter")
	var count uint64
	for code := 1; code <= 5; code++ {
		n := h.metrics.requests[code].Load()
		count += n
		fmt.Fprintf(w, "infra_rpc_requests_total{status_class=\"%dxx\"} %d\n", code, n)
	}
	fmt.Fprintln(w, "# TYPE infra_rpc_duration_seconds summary")
	fmt.Fprintf(w, "infra_rpc_duration_seconds_sum %f\ninfra_rpc_duration_seconds_count %d\n", float64(h.metrics.durationNS.Load())/1e9, count)
	fmt.Fprintf(w, "# TYPE infra_rpc_in_flight gauge\ninfra_rpc_in_flight %d\n", len(h.slots))
	fmt.Fprintf(w, "# TYPE infra_usage_admission_errors_total counter\ninfra_usage_admission_errors_total %d\n", h.metrics.admissionErrors.Load())
	fmt.Fprintf(w, "# TYPE infra_usage_completion_errors_total counter\ninfra_usage_completion_errors_total %d\n", h.metrics.completionErrors.Load())
}
