package middleware

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// statusWriter wraps http.ResponseWriter to capture the response status code.
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.status = http.StatusOK
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}

func (w *statusWriter) Flush() {
	if fl, ok := w.ResponseWriter.(http.Flusher); ok {
		fl.Flush()
	}
}

// RequestLogger logs every HTTP request with structured fields:
// method, path, status, duration_ms, client_ip, and user_id.
//
// Health check requests (/health) are logged at Debug level to reduce noise.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)

		duration := time.Since(start)
		userID := GetUserID(r.Context())

		attrs := []slog.Attr{
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", sw.status),
			slog.Float64("duration_ms", float64(duration.Microseconds())/1000.0),
			slog.String("client_ip", getClientIP(r)),
			slog.String("user_id", userID),
		}

		if r.URL.Path == "/health" {
			slog.LogAttrs(r.Context(), slog.LevelDebug, "request", attrs...)
		} else {
			slog.LogAttrs(r.Context(), slog.LevelInfo, "request", attrs...)
		}
	})
}
