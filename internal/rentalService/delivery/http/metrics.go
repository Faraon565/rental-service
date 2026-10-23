package http

import (
	"fmt"
	stdhttp "net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type metricKey struct {
	Method string
	Path   string
	Status string
}

type metricValue struct {
	Count       int64
	DurationSum float64
}

var httpMetrics = struct {
	sync.Mutex
	values map[metricKey]metricValue
}{
	values: make(map[metricKey]metricValue),
}

type responseWriter struct {
	stdhttp.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

func MetricsMiddleware(next stdhttp.Handler) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		start := time.Now()
		wrappedWriter := &responseWriter{
			ResponseWriter: w,
			statusCode:     stdhttp.StatusOK,
		}

		next.ServeHTTP(wrappedWriter, r)

		key := metricKey{
			Method: r.Method,
			Path:   r.URL.Path,
			Status: strconv.Itoa(wrappedWriter.statusCode),
		}

		httpMetrics.Lock()
		value := httpMetrics.values[key]
		value.Count++
		value.DurationSum += time.Since(start).Seconds()
		httpMetrics.values[key] = value
		httpMetrics.Unlock()
	})
}

func MetricsHandler() stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.Method != stdhttp.MethodGet {
			stdhttp.Error(w, "method not allowed", stdhttp.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		httpMetrics.Lock()
		keys := make([]metricKey, 0, len(httpMetrics.values))
		for key := range httpMetrics.values {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			return keys[i].Method+keys[i].Path+keys[i].Status < keys[j].Method+keys[j].Path+keys[j].Status
		})

		builder := strings.Builder{}
		builder.WriteString("# HELP http_requests_total Total number of HTTP requests.\n")
		builder.WriteString("# TYPE http_requests_total counter\n")

		for _, key := range keys {
			value := httpMetrics.values[key]
			builder.WriteString(fmt.Sprintf(
				"http_requests_total{method=%q,path=%q,status=%q} %d\n",
				key.Method,
				key.Path,
				key.Status,
				value.Count,
			))
		}

		builder.WriteString("\n# HELP http_request_duration_seconds_sum Total HTTP request duration in seconds.\n")
		builder.WriteString("# TYPE http_request_duration_seconds_sum counter\n")

		for _, key := range keys {
			value := httpMetrics.values[key]
			builder.WriteString(fmt.Sprintf(
				"http_request_duration_seconds_sum{method=%q,path=%q,status=%q} %.6f\n",
				key.Method,
				key.Path,
				key.Status,
				value.DurationSum,
			))
		}

		builder.WriteString("\n# HELP http_request_duration_seconds_count Total number of observed HTTP request durations.\n")
		builder.WriteString("# TYPE http_request_duration_seconds_count counter\n")

		for _, key := range keys {
			value := httpMetrics.values[key]
			builder.WriteString(fmt.Sprintf(
				"http_request_duration_seconds_count{method=%q,path=%q,status=%q} %d\n",
				key.Method,
				key.Path,
				key.Status,
				value.Count,
			))
		}
		httpMetrics.Unlock()

		_, _ = w.Write([]byte(builder.String()))
	})
}
