package main

import (
	"flag"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"gopkg.in/errgo.v1"
	"gopkg.in/hockeypuck/conflux.v2/recon"
	"gopkg.in/hockeypuck/hkp.v1/sks"
	log "gopkg.in/hockeypuck/logrus.v0"

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

	err = dump(settings)
	cmd.Die(err)
}

func dump(settings *server.Settings) error {
	st, err := server.DialStorage(settings)
	if err != nil {
		return errgo.Mask(err)
	}
	defer st.Close()

	ptree, err := sks.NewPrefixTree(settings.Conflux.Recon.LevelDB.Path, &settings.Conflux.Recon.Settings)
	if err != nil {
		return errgo.Mask(err)
	}
	err = ptree.Create()
	if err != nil {
		return errgo.Mask(err)
	}
	defer ptree.Close()

	root, err := ptree.Root()
	if err != nil {
		return errgo.Mask(err)
	}

	// Depth-first walk of the prefix tree
	nodes := []recon.PrefixNode{root}
	for len(nodes) > 0 {
		node := nodes[0]
		nodes = nodes[1:]

		if node.IsLeaf() {
			elements, err := node.Elements()
			if err != nil {
				return errgo.Mask(err)
			}
			for _, element := range elements {
				zb := element.Bytes()
				zb = recon.PadSksElement(zb)
				log.Printf("%x", zb)
			}
		} else {
			children, err := node.Children()
			if err != nil {
				return errgo.Mask(err)
			}
			nodes = append(nodes, children...)
		}
	}
	return nil
}
