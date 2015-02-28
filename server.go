package server

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
	"gopkg.in/errgo.v1"
	"gopkg.in/tomb.v2"

	"gopkg.in/hockeypuck/hkp.v0"
	"gopkg.in/hockeypuck/hkp.v0/sks"
	"gopkg.in/hockeypuck/hkp.v0/storage"
	"gopkg.in/hockeypuck/mgohkp.v0"
)

type Server struct {
	settings *Settings
	st       storage.Storage
	r        *httprouter.Router
	sksPeer  *sks.Peer

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
		s.st, err = mgohkp.Dial(settings.OpenPGP.DB.DSN)
		if err != nil {
			return nil, errgo.Mask(err)
		}
	default:
		return nil, errgo.Newf("storage driver %q not supported", settings.OpenPGP.DB.DSN)
	}

	h := hkp.NewHandler(s.st)
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
	s.t.Go(s.listenAndServeHKP)
	if s.settings.HKPS != nil {
		s.t.Go(s.listenAndServeHKPS)
	}

	if s.sksPeer != nil {
		s.sksPeer.Start()
	}

	return nil
}

func (s *Server) Wait() error {
	return s.t.Wait()
}

func (s *Server) Stop() {
	s.t.Go(func() error {
		s.sksPeer.Stop()
		return nil
	})
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
	return http.Serve(ln, s.r)
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
	return http.Serve(ln, s.r)
}

/*
func main() {
	session, err := mgo.Dial("localhost:27017")
	if err != nil {
		panic(err)
	}

	st, err := mgohkp.NewStorage(session)
	if err != nil {
		panic(err)
	}

	h := hkp.NewHandler(st)
	r := httprouter.New()
	h.Register(r)

	peer, err := sks.NewPeer(st, "recon-ptree", nil)
	if err != nil {
		panic(err)
	}
	peer.Start()

	http.ListenAndServe(":11371", r)
}
*/
