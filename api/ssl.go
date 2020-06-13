package api

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"golang.org/x/crypto/acme/autocert"

	"github.com/filebrowser/filebrowser/v3/log"
)

// sslMode defines ssl mode for Server server
type sslMode int8

const (
	// None defines to run http server only
	None sslMode = iota

	// Static defines to run both https and http server. Redirect http to https
	Static

	// Auto defines to run both https and http server. Redirect http to https. Https server with autocert support
	Auto
)

// SSLConfig holds all ssl params for Server server
type SSLConfig struct {
	SSLMode      sslMode
	Cert         string
	Key          string
	Port         int
	ACMELocation string
	ACMEEmail    string
}

// httpToHTTPSRouter creates new router which does redirect from http to https server
// with default middlewares. Used in 'static' ssl mode.
func (s *Server) httpToHTTPSRouter() chi.Router {
	log.Debugf("create https-to-http redirect routes")
	router := chi.NewRouter()
	router.Use(middleware.RealIP, RequestID, Recoverer)
	router.Use(middleware.Throttle(CurrentRequests), middleware.Timeout(60*time.Second)) //nolint:gomnd

	router.Handle("/*", s.redirectHandler())
	return router
}

// httpChallengeRouter creates new router which performs ACME "http-01" challenge response
// with default middlewares. This part is necessary to obtain certificate from LE.
// If it receives not a acme challenge it performs redirect to https server.
// Used in 'auto' ssl mode.
func (s *Server) httpChallengeRouter(m *autocert.Manager) chi.Router {
	log.Debugf("create http-challenge routes")
	router := chi.NewRouter()
	router.Use(middleware.RealIP, RequestID, Recoverer)
	router.Use(middleware.Throttle(CurrentRequests), middleware.Timeout(60*time.Second)) //nolint:gomnd

	router.Handle("/*", m.HTTPHandler(s.redirectHandler()))
	return router
}

func (s *Server) redirectHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		newURL := s.ServerURL + r.URL.Path
		if r.URL.RawQuery != "" {
			newURL += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, newURL, http.StatusTemporaryRedirect)
	})
}

func (s *Server) makeAutocertManager() *autocert.Manager {
	return &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache(s.SSLConfig.ACMELocation),
		HostPolicy: autocert.HostWhitelist(s.getServerHost()),
		Email:      s.SSLConfig.ACMEEmail,
	}
}

// makeHTTPSAutoCertServer makes https server with autocert mode (LE support)
func (s *Server) makeHTTPSAutocertServer(addr string, router http.Handler, m *autocert.Manager) *http.Server {
	server := s.makeHTTPServer(addr, router)
	cfg := s.makeTLSConfig()
	cfg.GetCertificate = m.GetCertificate
	server.TLSConfig = cfg
	return server
}

// makeHTTPSServer makes https server for static mode
func (s *Server) makeHTTPSServer(addr string, router http.Handler) *http.Server {
	server := s.makeHTTPServer(addr, router)
	server.TLSConfig = s.makeTLSConfig()
	return server
}

// getServerHost returns hostname for the server.
// For example for serverURL https://filebrowser.org:443 it should return filebrowser.org
func (s *Server) getServerHost() string {
	u, err := url.Parse(s.ServerURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func (s *Server) makeTLSConfig() *tls.Config {
	return &tls.Config{
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			// tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			// tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			// tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		},
		MinVersion: tls.VersionTLS12,
		CurvePreferences: []tls.CurveID{
			tls.CurveP256,
			tls.X25519,
			tls.CurveP384,
		},
	}
}
