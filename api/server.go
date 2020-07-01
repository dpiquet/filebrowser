package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/didip/tollbooth/v6"
	"github.com/didip/tollbooth_chi"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/cors"
	"github.com/go-pkgz/auth"
	"github.com/go-pkgz/lcw"

	"github.com/filebrowser/filebrowser/v3/log"
)

const (
	ShutdownTimeout      = 10 * time.Second
	CurrentRequests      = 1000
	AdminCurrentRequests = 10

	AuthRouterLimiter = 5

	OpenRoutesTimeout   = 5 * time.Second
	StaticRouterLimiter = 100

	ProtectedRoutesTimeout = 30 * time.Second
	ProtectedRouterLimiter = 10

	AdminRoutesTimeout = 30 * time.Second
	AdminRouterLimiter = 10
)

type Server struct {
	Authenticator   *auth.Service
	Store           Store
	Cache           LoadingCache
	Host            string
	Port            int
	ServerURL       string
	SharedSecret    string
	Revision        string
	EnableAccessLog bool

	SSLConfig   SSLConfig
	httpsServer *http.Server
	httpServer  *http.Server
	lock        sync.Mutex
}

type Store interface {
	protectedStore
	adminStore
}

// LoadingCache defines interface for caching
type LoadingCache interface {
	Get(key lcw.Key, fn func() ([]byte, error)) (data []byte, err error) // load from cache if found or put to cache and return
	Flush(req lcw.FlusherRequest)                                        // evict matched records
}

func (s *Server) Run() {
	httpAddr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	httpsAddr := fmt.Sprintf("%s:%d", s.Host, s.SSLConfig.Port)
	switch s.SSLConfig.SSLMode {
	case None:
		log.Infof("activate http rest server on %s", httpAddr)

		s.lock.Lock()
		s.httpServer = s.makeHTTPServer(httpAddr, s.routes())
		s.httpServer.ErrorLog = log.ToStdLogger(log.DefaultLogger, log.LevelWarn)
		s.lock.Unlock()

		err := s.httpServer.ListenAndServe()
		log.Warnf("http server terminated, %s", err)
	case Static:
		log.Infof("activate https server in 'static' mode on %s", httpsAddr)

		s.lock.Lock()
		s.httpsServer = s.makeHTTPSServer(httpsAddr, s.routes())
		s.httpsServer.ErrorLog = log.ToStdLogger(log.DefaultLogger, log.LevelWarn)

		s.httpServer = s.makeHTTPServer(httpAddr, s.httpToHTTPSRouter())
		s.httpServer.ErrorLog = log.ToStdLogger(log.DefaultLogger, log.LevelWarn)
		s.lock.Unlock()

		go func() {
			log.Infof("activate http redirect server on %s", httpAddr)
			err := s.httpServer.ListenAndServe()
			log.Warnf("http redirect server terminated, %s", err)
		}()

		err := s.httpsServer.ListenAndServeTLS(s.SSLConfig.Cert, s.SSLConfig.Key)
		log.Warnf("https server terminated, %s", err)
	case Auto:
		log.Infof("activate https server in 'auto' mode on %s", httpsAddr)

		m := s.makeAutocertManager()
		s.lock.Lock()
		s.httpsServer = s.makeHTTPSAutocertServer(httpsAddr, s.routes(), m)
		s.httpsServer.ErrorLog = log.ToStdLogger(log.DefaultLogger, log.LevelWarn)

		s.httpServer = s.makeHTTPServer(httpAddr, s.httpChallengeRouter(m))
		s.httpServer.ErrorLog = log.ToStdLogger(log.DefaultLogger, log.LevelWarn)

		s.lock.Unlock()

		go func() {
			log.Infof("activate http challenge server on %s", httpAddr)

			err := s.httpServer.ListenAndServe()
			log.Warnf("http challenge server terminated, %s", err)
		}()

		err := s.httpsServer.ListenAndServeTLS("", "")
		log.Warnf("https server terminated, %s", err)
	}
}

// Shutdown rest http server
func (s *Server) Shutdown() {
	log.Warnf("shutdown rest server")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s.lock.Lock()
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			log.Debugf("http shutdown error, %s", err)
		}
		log.Debugf("shutdown http server completed")
	}

	if s.httpsServer != nil {
		log.Warnf("shutdown https server")
		if err := s.httpsServer.Shutdown(ctx); err != nil {
			log.Debugf("https shutdown error, %s", err)
		}
		log.Debugf("shutdown https server completed")
	}
	s.lock.Unlock()
}

func (s *Server) makeHTTPServer(addr string, router http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
}

func (s *Server) routes() chi.Router {
	router := chi.NewRouter()
	router.Use(middleware.Throttle(CurrentRequests), middleware.RealIP, RequestID)
	if s.EnableAccessLog {
		router.Use(Logger)
	}
	router.Use(Recoverer)
	corsMiddleware := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-XSRF-Token", "X-JWT"},
		ExposedHeaders:   []string{"Authorization"},
		AllowCredentials: true,
		MaxAge:           300,
	})
	router.Use(corsMiddleware.Handler)

	publicHandlers, _, _ := s.makeHandlerGroups()

	router.NotFound(publicHandlers.indexHandler)
	router.Group(func(r chi.Router) {
		r.Use(middleware.Timeout(OpenRoutesTimeout))
		r.Use(tollbooth_chi.LimitHandler(tollbooth.NewLimiter(StaticRouterLimiter, nil)))
		r.Get("/static/*", publicHandlers.staticHandler)
	})

	authHandler, avatarHandler := s.Authenticator.Handlers()

	router.Group(func(r chi.Router) {
		r.Use(middleware.Timeout(OpenRoutesTimeout))
		r.Use(tollbooth_chi.LimitHandler(tollbooth.NewLimiter(AuthRouterLimiter, nil)), middleware.NoCache)
		r.Mount("/auth", authHandler)
	})

	authMiddleware := s.Authenticator.Middleware()

	// api routes
	router.Route("/api/v1", func(rapi chi.Router) {
		rapi.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(OpenRoutesTimeout))
			r.Use(tollbooth_chi.LimitHandler(tollbooth.NewLimiter(StaticRouterLimiter, nil)), middleware.NoCache)
			r.Mount("/avatar", avatarHandler)
		})

		// protected routes, require auth
		rapi.Group(func(rauth chi.Router) {
			rauth.Use(middleware.Timeout(ProtectedRoutesTimeout))
			rauth.Use(tollbooth_chi.LimitHandler(tollbooth.NewLimiter(ProtectedRouterLimiter, nil)))
			rauth.Use(authMiddleware.Auth, middleware.NoCache)
		})

		// admin routes, require auth and admin users only
		rapi.Route("/admin", func(radmin chi.Router) {
			radmin.Use(middleware.Timeout(AdminRoutesTimeout))
			radmin.Use(tollbooth_chi.LimitHandler(tollbooth.NewLimiter(AdminRouterLimiter, nil)))
			radmin.Use(authMiddleware.Auth, authMiddleware.AdminOnly)
		})
	})

	return router
}

func (s *Server) makeHandlerGroups() (*openHandlers, *protectedHandlers, *adminHandlers) {
	publicHandlers := &openHandlers{
		BasePath: s.getServerBasePath(),
		Revision: s.Revision,
	}
	return publicHandlers, &protectedHandlers{}, &adminHandlers{}
}

// getServerBasePath returns base path for the server.
// For example for serverURL https://filebrowser.org/base/path it should return /base/path
func (s *Server) getServerBasePath() string {
	u, err := url.Parse(s.ServerURL)
	if err != nil {
		return "/"
	}
	return u.Path
}
