package server

import (
	"net/http"
	"net/http/pprof"
	"os"
)

// registerPprof mounts net/http/pprof diagnostic endpoints conditionally when SQLFOX_PPROF=1.
// All endpoints are strictly guarded to loopback clients only.
func (s *Server) registerPprof(mux *http.ServeMux) {
	if os.Getenv("SQLFOX_PPROF") != "1" {
		return
	}

	loopbackOnly := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !isLoopback(r.RemoteAddr) {
				http.Error(w, "Forbidden: pprof is restricted to loopback localhost", http.StatusForbidden)
				return
			}
			h(w, r)
		}
	}

	mux.HandleFunc("GET /debug/pprof/", loopbackOnly(pprof.Index))
	mux.HandleFunc("GET /debug/pprof/cmdline", loopbackOnly(pprof.Cmdline))
	mux.HandleFunc("GET /debug/pprof/profile", loopbackOnly(pprof.Profile))
	mux.HandleFunc("GET /debug/pprof/symbol", loopbackOnly(pprof.Symbol))
	mux.HandleFunc("GET /debug/pprof/trace", loopbackOnly(pprof.Trace))
	mux.HandleFunc("GET /debug/pprof/heap", loopbackOnly(pprof.Handler("heap").ServeHTTP))
	mux.HandleFunc("GET /debug/pprof/goroutine", loopbackOnly(pprof.Handler("goroutine").ServeHTTP))
}
