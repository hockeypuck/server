package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"github.com/hockeypuck/server"
	"gopkg.in/errgo.v1"
)

var (
	configFile = flag.String("config", "", "config file")
)

func die(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, errgo.Details(err))
		os.Exit(1)
	}
	os.Exit(0)
}

func main() {
	flag.Parse()

	var (
		settings *server.Settings
		err      error
	)
	if configFile != nil {
		conf, err := ioutil.ReadFile(*configFile)
		if err != nil {
			die(errgo.Mask(err))
		}
		settings, err = server.ParseSettings(string(conf))
		if err != nil {
			die(errgo.Mask(err))
		}
	}

	srv, err := server.NewServer(settings)
	if err != nil {
		die(err)
	}

	srv.Start()

	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		for {
			select {
			case sig := <-c:
				switch sig {
				case syscall.SIGINT, syscall.SIGTERM:
					srv.Stop()
				default:
					srv.LogRotate()
				}
			}
		}
	}()

	err = srv.Wait()
	die(err)
}
