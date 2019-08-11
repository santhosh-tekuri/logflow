// Copyright 2019 Santhosh Kumar Tekuri
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
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

	k8s := filepath.Join(p.dir, "k8s")
	if !fileExists(k8s) {
		m := p.fetchMetadata()
		b, err := json.Marshal(m)
		if err != nil {
			panic(err)
		}
		if err := ioutil.WriteFile(k8s, b, 0700); err != nil {
			panic(err)
		}
	}

	c.watch(filepath.Dir(p.dfile))
	p.save()

	if strings.HasPrefix(filepath.Base(p.dir), "counter") {
		go parseLogs(p.dir)
	}
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
	logs := getLogFiles(p.dir)
	next := nextLogFile(logs[len(logs)-1])
	if err := ioutil.WriteFile(next, []byte("END\n"), 0700); err != nil {
		panic(err)
	}

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
