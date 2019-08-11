package main

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

const kdir = "/var/log/containers/"
const qdir = "/var/log/containers/flow/"

var exitCh = make(chan struct{})

func main() {
	mkdirs(qdir)
	c := openCollector()
	defer c.close()
	c.watch(kdir)
	for _, m := range glob(kdir, "*.log") {
		c.add(m)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.run()
	}()

	ch := make(chan os.Signal, 2)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch
}
