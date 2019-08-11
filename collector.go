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
	"sync"
	"time"
)

type collector struct {
	kfiles  map[string]*pod
	records chan<- record
	wg      sync.WaitGroup

	mu     sync.RWMutex
	dfiles map[string]*pod
}

func openCollector(records chan<- record) *collector {
	w := &collector{
		kfiles:  make(map[string]*pod),
		dfiles:  make(map[string]*pod),
		records: records,
	}
	return w
}

func (c *collector) close() {
	c.wg.Wait()
}

func (c *collector) add(kfile string) {
	dir := filepath.Join(qdir, strings.TrimSuffix(filepath.Base(kfile), ".log"))
	if !fileExists(kfile) {
		if !fileExists(filepath.Join(dir, ".terminated")) {
			c.markTerminated(dir)
		}
		c.runParser(dir)
		return
	}
	dfile := readLinks(kfile)
	p := &pod{
		kfile: kfile,
		dfile: dfile,
		dfi:   stat(dfile),
		dir:   dir,
	}
	mkdirs(p.dir)
	c.mu.Lock()
	c.dfiles[p.dfile] = p
	c.mu.Unlock()
	c.kfiles[p.kfile] = p

	k8s := filepath.Join(p.dir, ".k8s")
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

	p.save()
	c.runParser(p.dir)
}

func (c *collector) runParser(dir string) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer fmt.Println("parser", dir, "exited")
		parseLogs(dir, c.records)
	}()
}

func (c *collector) markTerminated(dir string) {
	logs := getLogFiles(dir)
	if len(logs) == 0 {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Println(err)
		}
		return
	}
	next := nextLogFile(logs[len(logs)-1])
	if err := ioutil.WriteFile(next, []byte("END\n"), 0700); err != nil {
		panic(err)
	}
	f, err := os.Create(filepath.Join(dir, ".terminated"))
	if err != nil {
		panic(err)
	}
	_ = f.Close()
}

func (c *collector) terminated(kfile string) {
	p, ok := c.kfiles[kfile]
	if !ok {
		panic("pod not found for " + kfile)
	}
	c.markTerminated(p.dir)
	//c.unwatch(filepath.Dir(p.dfile))
	delete(c.kfiles, p.kfile)
	c.mu.Lock()
	delete(c.dfiles, p.dfile)
	c.mu.Unlock()
}

func (c *collector) run() {
	for {
		select {
		case <-exitCh:
			return
		case <-time.After(250 * time.Millisecond):
			c.mu.RLock()
			// fmt.Println("checking...")
			for name, p := range c.dfiles {
				fi, err := os.Stat(name)
				if err != nil {
					fmt.Println(err)
					continue
				}
				if !os.SameFile(p.dfi, fi) {
					p.dfi = fi
					// fmt.Println(name, "file rotated")
					p.save()
					// fmt.Println(name, "file rotated notified")
				}
			}
			c.mu.RUnlock()
		}
	}
}

func kfile(name string) bool {
	return strings.HasPrefix(name, kdir) && strings.HasSuffix(name, ".log")
}
