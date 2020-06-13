package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/go-pkgz/auth"
	"github.com/go-pkgz/auth/avatar"
	"github.com/go-pkgz/auth/provider"
	"github.com/go-pkgz/auth/token"
	cache "github.com/go-pkgz/lcw"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"

	"github.com/filebrowser/filebrowser/v3/api"
	"github.com/filebrowser/filebrowser/v3/log"
)

// ServerCommand with command line flags and env
type ServerCommand struct {
	Auth   AuthGroup   `group:"auth" namespace:"auth" env-namespace:"AUTH"`
	Avatar AvatarGroup `group:"avatar" namespace:"avatar" env-namespace:"AVATAR"`
	Cache  CacheGroup  `group:"cache" namespace:"cache" env-namespace:"CACHE"`
	Store  StoreGroup  `group:"store" namespace:"store" env-namespace:"STORE"`
	SSL    SSLGroup    `group:"ssl" namespace:"ssl" env-namespace:"SSL"`

	AdminPasswd     string `long:"admin-passwd" env:"ADMIN_PASSWD" default:"" description:"admin basic auth password"`
	EnableAccessLog bool   `long:"enable-access-log" env:"ENABLE_ACCESS_LOG" description:"enable access log"`
	RootPath        string `long:"root" env:"ROOT_PATH" default:"." description:"root folder"`
	Host            string `long:"host" env:"HOST" default:"0.0.0.0" description:"host"`
	Port            int    `long:"port" env:"PORT" default:"8080" description:"port"`

	CommonOpts
}

// AuthGroup defines options group store params
type StoreGroup struct {
	Type string `long:"type" env:"TYPE" description:"type of storage" choice:"bolt" default:"bolt"`
	Bolt struct {
		Path    string        `long:"path" env:"PATH" default:".var/" description:"parent dir for bolt files"`
		Timeout time.Duration `long:"timeout" env:"TIMEOUT" default:"30s" description:"bolt timeout"`
	} `group:"bolt" namespace:"bolt" env-namespace:"BOLT"`
}

// AuthGroup defines options group auth params
type AuthGroup struct {
	TTL struct {
		JWT    time.Duration `long:"jwt" env:"JWT" default:"10s" description:"jwt TTL"`
		Cookie time.Duration `long:"cookie" env:"COOKIE" default:"200h" description:"auth cookie TTL"`
	} `group:"ttl" namespace:"ttl" env-namespace:"TTL"`
	Google    OAuthGroup `group:"google" namespace:"google" env-namespace:"GOOGLE" description:"Google OAuth"`
	Github    OAuthGroup `group:"github" namespace:"github" env-namespace:"GITHUB" description:"Github OAuth"`
	Facebook  OAuthGroup `group:"facebook" namespace:"facebook" env-namespace:"FACEBOOK" description:"Facebook OAuth"`
	Yandex    OAuthGroup `group:"yandex" namespace:"yandex" env-namespace:"YANDEX" description:"Yandex OAuth"`
	Twitter   OAuthGroup `group:"twitter" namespace:"twitter" env-namespace:"TWITTER" description:"Twitter OAuth"`
	Dev       bool       `long:"dev" env:"DEV" description:"enable dev (local) oauth2"`
	Anonymous bool       `long:"anon" env:"ANON" description:"enable anonymous login"`
	Admin     struct {
		Username string `long:"username" env:"USERNAME" default:"admin" description:"admin username"`
		Password string `long:"password" env:"PASSWORD" default:"admin" description:"admin password"`
	} `group:"admin" namespace:"admin" env-namespace:"ADMIN"`
}

// OAuthGroup defines options group for oauth params
type OAuthGroup struct {
	CID  string `long:"cid" env:"CID" description:"OAuth client ID"`
	CSEC string `long:"csec" env:"CSEC" description:"OAuth client secret"`
}

// AvatarGroup defines options group for avatar params
type AvatarGroup struct {
	Type string `long:"type" env:"TYPE" description:"type of avatar storage" choice:"fs" choice:"bolt" choice:"uri" default:"fs"` //nolint
	FS   struct {
		Path string `long:"path" env:"PATH" default:"./var/avatars" description:"avatars location"`
	} `group:"fs" namespace:"fs" env-namespace:"FS"`
	Bolt struct {
		File string `long:"file" env:"FILE" default:"./var/avatars.db" description:"avatars bolt file location"`
	} `group:"bolt" namespace:"bolt" env-namespace:"bolt"`
	URI    string `long:"uri" env:"URI" default:"./var/avatars" description:"avatar's store URI"`
	RszLmt int    `long:"rsz-lmt" env:"RESIZE" default:"0" description:"max image size for resizing avatars on save"`
}

// CacheGroup defines options group for cache params
type CacheGroup struct {
	Type string `long:"type" env:"TYPE" description:"type of cache" choice:"mem" choice:"none" default:"mem"` // nolint
	Max  struct {
		Items int   `long:"items" env:"ITEMS" default:"1000" description:"max cached items"`
		Value int   `long:"value" env:"VALUE" default:"65536" description:"max size of cached value"`
		Size  int64 `long:"size" env:"SIZE" default:"50000000" description:"max size of total cache"`
	} `group:"max" namespace:"max" env-namespace:"MAX"`
}

// SSLGroup defines options group for server ssl params
type SSLGroup struct {
	Type         string `long:"type" env:"TYPE" description:"ssl (auto) support" choice:"none" choice:"static" choice:"auto" default:"none"` //nolint
	Port         int    `long:"port" env:"PORT" description:"port number for https server" default:"8443"`
	Cert         string `long:"cert" env:"CERT" description:"path to cert.pem file"`
	Key          string `long:"key" env:"KEY" description:"path to key.pem file"`
	ACMELocation string `long:"acme-location" env:"ACME_LOCATION" description:"dir where certificates will be stored by autocert manager" default:"./var/acme"` //nolint
	ACMEEmail    string `long:"acme-email" env:"ACME_EMAIL" description:"admin email for certificate notifications"`
}

// LoadingCache defines interface for caching
type LoadingCache interface {
	Get(key cache.Key, fn func() ([]byte, error)) (data []byte, err error) // load from cache if found or put to cache and return
	Flush(req cache.FlusherRequest)                                        // evict matched records
	Close() error
}

// serverApp holds all active objects
type serverApp struct {
	*ServerCommand
	restSrv    *api.Server
	terminated chan struct{}
}

// Execute runs file browser server
func (s *ServerCommand) Execute(_ []string) error {
	resetEnv("AUTH_ADMIN_USERNAME", "AUTH_ADMIN_PASSWORD")

	ctx, cancel := context.WithCancel(context.Background())
	go func() { // catch signal and invoke graceful termination
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop
		log.Warnf("interrupt signal")
		cancel()
	}()

	app, err := s.newServerApp()
	if err != nil {
		log.Fatalf("failed to setup application, %+v", err)
		return err
	}
	if err = app.run(ctx); err != nil {
		log.Fatalf("terminated with error %+v", err)
		return err
	}
	log.Infof("terminated")
	return nil
}

// newServerApp prepares application and return it with all active parts
// doesn't start anything
func (s *ServerCommand) newServerApp() (*serverApp, error) {
	loadingCache, err := s.makeCache()
	if err != nil {
		return nil, errors.Wrap(err, "failed to make cache")
	}

	avatarStore, err := s.makeAvatarStore()
	if err != nil {
		return nil, errors.Wrap(err, "failed to make avatar store")
	}
	authRefreshCache := newAuthRefreshCache()
	authenticator, err := s.makeAuthenticator(avatarStore, authRefreshCache)
	if err != nil {
		return nil, errors.Wrap(err, "failed to make authenticator")
	}

	sslConfig, err := s.makeSSLConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to make config of ssl server params")
	}

	apiServer := &api.Server{
		Authenticator:   authenticator,
		Cache:           loadingCache,
		Host:            s.Host,
		Port:            s.Port,
		ServerURL:       s.ServerURL,
		SharedSecret:    s.SharedSecret,
		Revision:        s.Revision,
		EnableAccessLog: s.EnableAccessLog,
		SSLConfig:       sslConfig,
	}

	return &serverApp{
		ServerCommand: s,
		restSrv:       apiServer,
		terminated:    make(chan struct{}),
	}, nil
}

func (s *ServerCommand) makeAuthenticator(avatarStore avatar.Store, authRefreshCache *authRefreshCache) (*auth.Service, error) { //nolint:interfacer,lll
	authenticator := auth.NewService(auth.Opts{
		URL:            strings.TrimSuffix(s.ServerURL, "/"),
		Issuer:         "filebrowser",
		TokenDuration:  s.Auth.TTL.JWT,
		CookieDuration: s.Auth.TTL.Cookie,
		SecureCookies:  strings.HasPrefix(s.ServerURL, "https://"),
		SecretReader: token.SecretFunc(func(aud string) (string, error) {
			if s.SharedSecret == "" {
				log.Infof("shared secret: %s", s.SharedSecret)
				return "", errors.New("shared secret is not provided")
			}
			return s.SharedSecret, nil
		}),
		ClaimsUpd: token.ClaimsUpdFunc(func(c token.Claims) token.Claims { // set attributes, on new token or refresh
			if c.User == nil {
				return c
			}
			//nolint:gocritic
			//c.User.SetAdmin(ds.IsAdmin(c.User.ID))
			c.User.SetBoolAttr("blocked", false)
			if strings.HasPrefix(c.User.ID, "anonymous_") {
				c.User.SetBoolAttr("anonymous", true)
			}
			fmt.Println("ClaimsUpd")
			return c
		}),
		AdminPasswd: s.AdminPasswd,
		Validator: token.ValidatorFunc(func(token string, claims token.Claims) bool { // check on each auth call (in middleware)
			fmt.Println("Validator")
			if claims.User == nil {
				return false
			}
			return !claims.User.BoolAttr("blocked")
		}),
		AvatarResizeLimit: s.Avatar.RszLmt,
		AvatarRoutePath:   "/api/v1/avatar",
		AvatarStore:       avatarStore,
		Logger:            log.NewLogrAdapter(log.DefaultLogger),
		RefreshCache:      authRefreshCache,
		UseGravatar:       true,
	})

	if err := s.addAuthProviders(authenticator); err != nil {
		return nil, err
	}

	return authenticator, nil
}

//nolint:gocyclo,unparam
func (s *ServerCommand) addAuthProviders(authenticator *auth.Service) error {
	providers := 0

	providers++
	authenticator.AddDirectProvider("local", provider.CredCheckerFunc(func(user, password string) (ok bool, err error) {
		if user != s.Auth.Admin.Username || password != s.Auth.Admin.Password {
			return false, errors.New("incorrect user credentials")
		}
		return true, nil
	}))

	if s.Auth.Google.CID != "" && s.Auth.Google.CSEC != "" {
		authenticator.AddProvider("google", s.Auth.Google.CID, s.Auth.Google.CSEC)
		providers++
	}
	if s.Auth.Github.CID != "" && s.Auth.Github.CSEC != "" {
		authenticator.AddProvider("github", s.Auth.Github.CID, s.Auth.Github.CSEC)
		providers++
	}
	if s.Auth.Facebook.CID != "" && s.Auth.Facebook.CSEC != "" {
		authenticator.AddProvider("facebook", s.Auth.Facebook.CID, s.Auth.Facebook.CSEC)
		providers++
	}
	if s.Auth.Yandex.CID != "" && s.Auth.Yandex.CSEC != "" {
		authenticator.AddProvider("yandex", s.Auth.Yandex.CID, s.Auth.Yandex.CSEC)
		providers++
	}
	if s.Auth.Twitter.CID != "" && s.Auth.Twitter.CSEC != "" {
		authenticator.AddProvider("twitter", s.Auth.Twitter.CID, s.Auth.Twitter.CSEC)
		providers++
	}

	if s.Auth.Dev {
		log.Infof("dev access enabled")
		authenticator.AddProvider("dev", "", "")
		providers++
	}

	if s.Auth.Anonymous {
		log.Infof("anonymous access enabled")
		authenticator.AddDirectProvider("anonymous", provider.CredCheckerFunc(func(user, _ string) (ok bool, err error) {
			if user != "anonymous" {
				return false, errors.Wrap(err, "username must be anonymous")
			}
			return true, nil
		}))
	}

	if providers == 0 {
		log.Warnf("no auth providers defined")
	}

	return nil
}

func (s *ServerCommand) makeAvatarStore() (avatar.Store, error) {
	log.Infof("make avatar store, type=%s", s.Avatar.Type)

	switch s.Avatar.Type {
	case "fs":
		if err := makeDirs(s.Avatar.FS.Path); err != nil {
			return nil, errors.Wrap(err, "failed to create avatar store")
		}
		return avatar.NewLocalFS(s.Avatar.FS.Path), nil
	case "bolt":
		if err := makeDirs(path.Dir(s.Avatar.Bolt.File)); err != nil {
			return nil, errors.Wrap(err, "failed to create avatar store")
		}
		return avatar.NewBoltDB(s.Avatar.Bolt.File, bolt.Options{})
	case "uri":
		return avatar.NewStore(s.Avatar.URI)
	}
	return nil, errors.Errorf("unsupported avatar store type %s", s.Avatar.Type)
}

func (s *ServerCommand) makeCache() (LoadingCache, error) {
	log.Infof("make cache, type=%s", s.Cache.Type)
	switch s.Cache.Type {
	case "mem":
		backend, err := cache.NewLruCache(cache.MaxCacheSize(s.Cache.Max.Size), cache.MaxValSize(s.Cache.Max.Value),
			cache.MaxKeys(s.Cache.Max.Items))
		if err != nil {
			return nil, errors.Wrap(err, "cache backend initialization")
		}
		return cache.NewScache(backend), nil
	case "none": //nolint:goconst
		return cache.NewScache(&cache.Nop{}), nil
	}
	return nil, errors.Errorf("unsupported cache type %s", s.Cache.Type)
}

func (s *ServerCommand) makeSSLConfig() (config api.SSLConfig, err error) {
	switch s.SSL.Type {
	case "none":
		config.SSLMode = api.None
	case "static":
		if s.SSL.Cert == "" {
			return config, errors.New("path to cert.pem is required")
		}
		if s.SSL.Key == "" {
			return config, errors.New("path to key.pem is required")
		}
		config.SSLMode = api.Static
		config.Port = s.SSL.Port
		config.Cert = s.SSL.Cert
		config.Key = s.SSL.Key
	case "auto":
		config.SSLMode = api.Auto
		config.Port = s.SSL.Port
		config.ACMELocation = s.SSL.ACMELocation
		if s.SSL.ACMEEmail != "" {
			config.ACMEEmail = s.SSL.ACMEEmail
		} else if u, e := url.Parse(s.ServerURL); e == nil {
			config.ACMEEmail = "admin@" + u.Hostname()
		}
	}
	return config, err
}

// Run all application objects
func (a *serverApp) run(ctx context.Context) error {
	go func() {
		// shutdown on context cancellation
		<-ctx.Done()
		log.Warnf("shutdown initiated")
		a.restSrv.Shutdown()
	}()

	a.restSrv.Run()

	close(a.terminated)
	return nil
}

// Wait for application completion (termination)
func (a *serverApp) Wait() {
	<-a.terminated
}

// authRefreshCache used by authenticator to minimize repeatable token refreshes
type authRefreshCache struct {
	cache.LoadingCache
}

func newAuthRefreshCache() *authRefreshCache {
	expirableCache, _ := cache.NewExpirableCache(cache.TTL(5 * time.Minute)) //nolint:gomnd
	return &authRefreshCache{LoadingCache: expirableCache}
}

// Get implements cache getter with key converted to string
func (c *authRefreshCache) Get(key interface{}) (interface{}, bool) {
	return c.LoadingCache.Peek(key.(string))
}

// Set implements cache setter with key converted to string
func (c *authRefreshCache) Set(key, value interface{}) {
	_, _ = c.LoadingCache.Get(key.(string), func() (cache.Value, error) { return value, nil })
}
