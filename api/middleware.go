package api

import (
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/middleware"

	"github.com/filebrowser/filebrowser/v3/log"
)

// Recoverer is a middleware that recovers from panics, logs the panic and returns a HTTP 500 status if possible.
func Recoverer(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				log.WithContext(r.Context()).Criticalf("request panic, %v", rvr)
				log.WithContext(r.Context()).Criticalf(string(debug.Stack()))
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		h.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

func Logger(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		startTime := time.Now()
		defer func() {
			log.WithContext(r.Context()).WithFields(log.Fields{
				"request_method":   r.Method,
				"request_path":     r.URL.Path,
				"request_duration": time.Since(startTime).String(),
				"response_status":  ww.Status(),
				"response_size":    ww.BytesWritten(),
			}).Infof("")
		}()

		h.ServeHTTP(ww, r)
	}
	return http.HandlerFunc(fn)
}
