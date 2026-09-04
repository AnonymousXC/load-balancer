package middleware

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
)

// common for both fasthttp and net
type SizeLimitConfig struct {
	MaxRequestSize  int64
	MaxResponseSize int64
}

func DefaultSizeLimitConfig() SizeLimitConfig {
	return SizeLimitConfig{
		MaxRequestSize:  10 * 1024 * 1024,
		MaxResponseSize: 10 * 1024 * 1024,
	}
}

func SizeLimitMiddleware(config SizeLimitConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if config.MaxRequestSize > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, config.MaxRequestSize)
			}

			if config.MaxResponseSize > 0 {
				limitedWriter := &limitedResponseWriter{
					ResponseWriter: w,
					maxSize:        config.MaxResponseSize,
					written:        0,
				}
				next.ServeHTTP(limitedWriter, r)
			} else {
				next.ServeHTTP(w, r)
			}
		})
	}
}

type limitedResponseWriter struct {
	http.ResponseWriter
	maxSize  int64
	written  int64
	status   int
	hijacked bool
}

func (lw *limitedResponseWriter) Write(b []byte) (int, error) {
	if lw.hijacked {
		return 0, fmt.Errorf("connection hijacked")
	}

	if lw.maxSize > 0 {
		if lw.written+int64(len(b)) > lw.maxSize {
			lw.WriteHeader(http.StatusRequestEntityTooLarge)
			return 0, fmt.Errorf("response too large")
		}
	}

	n, err := lw.ResponseWriter.Write(b)
	lw.written += int64(n)
	return n, err
}

func (lw *limitedResponseWriter) WriteHeader(statusCode int) {
	if lw.status == 0 {
		lw.status = statusCode
	}
	lw.ResponseWriter.WriteHeader(statusCode)
}

func (lw *limitedResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := lw.ResponseWriter.(http.Hijacker); ok {
		conn, rw, err := hj.Hijack()
		if err == nil {
			lw.hijacked = true
		}
		return conn, rw, err
	}
	return nil, nil, http.ErrNotSupported
}

func (lw *limitedResponseWriter) Flush() {
	if flusher, ok := lw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (lw *limitedResponseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := lw.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (lw *limitedResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	if rf, ok := lw.ResponseWriter.(io.ReaderFrom); ok {
		n, err := rf.ReadFrom(r)
		lw.written += n
		return n, err
	}
	return io.Copy(lw, r)
}
