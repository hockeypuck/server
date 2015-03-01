package server

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/carbocation/interpose"
	"github.com/julienschmidt/httprouter"
	"gopkg.in/errgo.v1"
	"gopkg.in/tomb.v2"

	"gopkg.in/hockeypuck/hkp.v0"
	"gopkg.in/hockeypuck/hkp.v0/sks"
	"gopkg.in/hockeypuck/hkp.v0/storage"
	log "gopkg.in/hockeypuck/logrus.v0"
	"gopkg.in/hockeypuck/mgohkp.v0"
)

type Server struct {
	settings  *Settings
	st        storage.Storage
	middle    *interpose.Middleware
	r         *httprouter.Router
	sksPeer   *sks.Peer
	logWriter io.WriteCloser

	t                 tomb.Tomb
	hkpAddr, hkpsAddr string
}

func NewServer(settings *Settings) (*Server, error) {
	if settings == nil {
		defaults := DefaultSettings()
		settings = &defaults
	}
	s := &Server{
		settings: settings,
		r:        httprouter.New(),
	}

	var err error
	switch settings.OpenPGP.DB.Driver {
	case "mongo":
		var options []mgohkp.Option
		if settings.OpenPGP.DB.Mongo.DB != "" {
			options = append(options, mgohkp.DBName(settings.OpenPGP.DB.Mongo.DB))
		}
		if settings.OpenPGP.DB.Mongo.Collection != "" {
			options = append(options, mgohkp.CollectionName(settings.OpenPGP.DB.Mongo.Collection))
		}
		s.st, err = mgohkp.Dial(settings.OpenPGP.DB.DSN, options...)
		if err != nil {
			return nil, errgo.Mask(err)
		}
	default:
		return nil, errgo.Newf("storage driver %q not supported", settings.OpenPGP.DB.DSN)
	}

	s.middle = interpose.New()
	s.middle.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			start := time.Now()
			next.ServeHTTP(rw, req)
			log.WithFields(log.Fields{
				req.Method: req.URL.String(),
				"duration": time.Since(start).String(),
				"from":     req.RemoteAddr,
			}).Info()
		})
	})
	s.middle.UseHandler(s.r)

	h, err := hkp.NewHandler(s.st)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	h.Register(s.r)

	if len(settings.Conflux.Recon.Partners) > 0 {
		s.sksPeer, err = sks.NewPeer(s.st, settings.Conflux.Recon.LevelDB.Path, &settings.Conflux.Recon.Settings)
		if err != nil {
			return nil, errgo.Mask(err)
		}
	}

	return s, nil
}

func (s *Server) Start() error {
	s.openLog()

	s.t.Go(s.listenAndServeHKP)
	if s.settings.HKPS != nil {
		s.t.Go(s.listenAndServeHKPS)
	}

	if s.sksPeer != nil {
		s.sksPeer.Start()
	}

	return nil
}

type nopCloser struct {
	io.Writer
}

func (nopCloser) Close() error { return nil }

func (s *Server) openLog() {
	defer func() {
		level, err := log.ParseLevel(strings.ToLower(s.settings.LogLevel))
		if err != nil {
			log.Warningf("invalid LogLevel=%q: %v", s.settings.LogLevel, err)
			return
		}
		log.SetLevel(level)
	}()

	s.logWriter = nopCloser{os.Stderr}
	if s.settings.LogFile != "" {
		f, err := os.OpenFile(s.settings.LogFile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			log.Errorf("failed to open LogFile=%q: %v", s.settings.LogFile, err)
		}
		s.logWriter = f
	}
	log.SetOutput(s.logWriter)
	log.Debug("log opened")
}

func (s *Server) closeLog() {
	log.SetOutput(os.Stderr)
	s.logWriter.Close()
}

func (s *Server) LogRotate() {
	w := s.logWriter
	s.openLog()
	w.Close()
}

func (s *Server) Wait() error {
	return s.t.Wait()
}

func (s *Server) Stop() {
	defer s.closeLog()

	s.t.Go(func() error {
		s.sksPeer.Stop()
		return nil
	})
	s.t.Kill(nil)
	s.t.Wait()
}

// tcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by listenAndServe and listenAndServeTLS so
// dead TCP connections (e.g. closing laptop mid-download) eventually
// go away.
type tcpKeepAliveListener struct {
	*net.TCPListener
}

// Accept implements net.Listener.
func (ln tcpKeepAliveListener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, errgo.Mask(err)
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}

var newListener = (*Server).newListener

func (s *Server) newListener(addr string) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, errgo.Mask(err)
	}

	s.t.Go(func() error {
		<-s.t.Dying()
		return ln.Close()
	})
	return tcpKeepAliveListener{ln.(*net.TCPListener)}, nil
}

func (s *Server) listenAndServeHKP() error {
	ln, err := newListener(s, s.settings.HKP.Bind)
	if err != nil {
		return err
	}
	s.hkpAddr = ln.Addr().String()
	return http.Serve(ln, s.middle)
}

func (s *Server) listenAndServeHKPS() error {
	config := &tls.Config{
		NextProtos: []string{"http/1.1"},
	}
	var err error
	config.Certificates = make([]tls.Certificate, 1)
	config.Certificates[0], err = tls.LoadX509KeyPair(s.settings.HKPS.Cert, s.settings.HKPS.Key)
	if err != nil {
		return errgo.Notef(err, "failed to load HKPS certificate=%q key=%q", s.settings.HKPS.Cert, s.settings.HKPS.Key)
	}

	ln, err := newListener(s, s.settings.HKP.Bind)
	if err != nil {
		return errgo.Mask(err)
	}
	s.hkpsAddr = ln.Addr().String()
	ln = tls.NewListener(ln, config)
	return http.Serve(ln, s.middle)
}
