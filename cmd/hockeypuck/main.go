package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"syscall"
	"time"

	log "gopkg.in/hockeypuck/logrus.v0"

	"github.com/hockeypuck/server"
	"gopkg.in/errgo.v1"
)

var (
	configFile = flag.String("config", "", "config file")
	cpuProf    = flag.Bool("cpuprof", false, "enable CPU profiling")
	memProf    = flag.Bool("memprof", false, "enable mem profiling")
)

func die(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, errgo.Details(err))
		os.Exit(1)
	}
	os.Exit(0)
}

func startCPUProf(prior *os.File) *os.File {
	if prior != nil {
		pprof.StopCPUProfile()
		log.Infof("CPU profile written to %q", prior.Name())
		prior.Close()
		os.Rename(filepath.Join(os.TempDir(), "hockeypuck-cpu.prof.part"),
			filepath.Join(os.TempDir(), "hockeypuck-cpu.prof"))
	}
	if *cpuProf {
		profName := filepath.Join(os.TempDir(), "hockeypuck-cpu.prof.part")
		f, err := os.Create(profName)
		if err != nil {
			die(errgo.Mask(err))
		}
		pprof.StartCPUProfile(f)
		return f
	}
	return nil
}

func writeMemProf() {
	if *memProf {
		tmpName := filepath.Join(os.TempDir(), fmt.Sprintf("hockeypuck-mem.prof.%d", time.Now().Unix()))
		profName := filepath.Join(os.TempDir(), "hockeypuck-mem.prof")
		f, err := os.Create(tmpName)
		if err != nil {
			die(errgo.Mask(err))
		}
		err = pprof.WriteHeapProfile(f)
		f.Close()
		if err != nil {
			log.Warningf("failed to write heap profile: %v", err)
			return
		}
		log.Infof("Heap profile written to %q", f.Name())
		os.Rename(tmpName, profName)
	}
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

	cpuFile := startCPUProf(nil)

	srv, err := server.NewServer(settings)
	if err != nil {
		die(err)
	}

	srv.Start()

	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1, syscall.SIGUSR2)
	go func() {
		for {
			select {
			case sig := <-c:
				switch sig {
				case syscall.SIGINT, syscall.SIGTERM:
					srv.Stop()
				case syscall.SIGUSR1:
					srv.LogRotate()
				case syscall.SIGUSR2:
					cpuFile = startCPUProf(cpuFile)
					writeMemProf()
				}
			}
		}
	}()

	err = srv.Wait()
	die(err)
}
