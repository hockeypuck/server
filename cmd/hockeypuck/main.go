package main

import (
	"os"
	"os/signal"
	"syscall"

	"gopkg.in/hockeypuck/server"
)

func main() {
	srv, err := server.NewServer(nil)
	if err != nil {
		panic(err)
	}

	srv.Start()

	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for {
			select {
			case sig := <-c:
				switch sig {
				case syscall.SIGINT, syscall.SIGTERM:
					srv.Stop()
				}
			}
		}
	}()

	err = srv.Wait()
	if err != nil {
		panic(err)
	}
}
