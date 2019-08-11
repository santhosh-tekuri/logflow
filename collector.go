package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

type collector struct {
	watcher *fsnotify.Watcher
	kfiles  map[string]*pod
	dfiles  map[string]*pod
}

func openCollector() *collector {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}
	w := &collector{
		watcher: fw,
		kfiles:  make(map[string]*pod),
		dfiles:  make(map[string]*pod),
	}
	return w
}

func (c *collector) close() error {
	return c.watcher.Close()
}

func (c *collector) watch(name string) {
	if err := c.watcher.Add(name); err != nil {
		panic(err)
	}
}

func (c *collector) unwatch(name string) {
	if err := c.watcher.Remove(name); err != nil {
		panic(err)
	}
}

func (c *collector) add(kfile string) {
	p := &pod{
		kfile: kfile,
		dfile: readLinks(kfile),
		dir:   filepath.Join(qdir, strings.TrimSuffix(filepath.Base(kfile), ".log")),
	}
	mkdirs(p.dir)
	c.dfiles[p.dfile] = p
	c.kfiles[p.kfile] = p

	c.watch(filepath.Dir(p.dfile))
	p.save()
}

func (c *collector) terminated(kfile string) {
	p, ok := c.kfiles[kfile]
	if !ok {
		panic("pod not found for " + kfile)
	}
	f, err := os.Create(filepath.Join(p.dir, ".terminated"))
	if err != nil {
		panic(err)
	}
	_ = f.Close()
	c.unwatch(filepath.Dir(p.dfile))
	delete(c.kfiles, p.kfile)
	delete(c.dfiles, p.dfile)
}

func (c *collector) run() {
	for {
		select {
		case <-exitCh:
			return
		case event := <-c.watcher.Events:
			switch event.Op {
			case fsnotify.Create:
				if kfile(event.Name) {
					fmt.Println(event)
					c.add(event.Name)
				} else if p, ok := c.dfiles[event.Name]; ok {
					fmt.Println("xxx", inode(event.Name), p.kfile, event.Op)
					p.save()
				}
			case fsnotify.Remove:
				if kfile(event.Name) {
					fmt.Println(event)
					c.terminated(event.Name)
				}
			}
		case err := <-c.watcher.Errors:
			fmt.Println(err)
		}
	}
}

func kfile(name string) bool {
	return strings.HasPrefix(name, kdir) && strings.HasSuffix(name, ".log")
}
