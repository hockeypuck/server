package main

import (
	"flag"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"gopkg.in/errgo.v1"
	"gopkg.in/hockeypuck/hkp.v0/sks"

	"github.com/hockeypuck/server"
	"github.com/hockeypuck/server/cmd"
)

var (
	configFile = flag.String("config", "", "config file")
	cpuProf    = flag.Bool("cpuprof", false, "enable CPU profiling")
	memProf    = flag.Bool("memprof", false, "enable mem profiling")
)

func main() {
	flag.Parse()

	var (
		settings *server.Settings
		err      error
	)
	if configFile != nil {
		conf, err := ioutil.ReadFile(*configFile)
		if err != nil {
			cmd.Die(errgo.Mask(err))
		}
		settings, err = server.ParseSettings(string(conf))
		if err != nil {
			cmd.Die(errgo.Mask(err))
		}
	}

	cpuFile := cmd.StartCPUProf(*cpuProf, nil)

	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGUSR2)
	go func() {
		for {
			select {
			case sig := <-c:
				switch sig {
				case syscall.SIGUSR2:
					cpuFile = cmd.StartCPUProf(*cpuProf, cpuFile)
					cmd.WriteMemProf(*memProf)
				}
			}
		}
	}()

	err = pbuild(settings)
	cmd.Die(err)
}

func pbuild(settings *server.Settings) error {
	st, err := server.DialStorage(settings)
	if err != nil {
		return errgo.Mask(err)
	}
	sksPeer, err := sks.NewPeer(st, settings.Conflux.Recon.LevelDB.Path, &settings.Conflux.Recon.Settings)
	if err != nil {
		return errgo.Mask(err)
	}
	defer sksPeer.Close()
	defer sksPeer.WriteStats()

	err = st.RenotifyAll()
	return errgo.Mask(err)
}
