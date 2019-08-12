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
	"fmt"
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

	var wg sync.WaitGroup

	tail := &tail{m: make(map[string]*logRef)}
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer fmt.Println("tailing exited")
		tail.run()
	}()

	r := newRecords()
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer fmt.Println("exporter exited")
		export(r)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		watchContainers(tail, r.records)
	}()

	ch := make(chan os.Signal, 2)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch
	close(exitCh)
	wg.Wait()
}

// func watchKDir(c *collector) {
// 	w, err := fsnotify.NewWatcher()
// 	if err != nil {
// 		panic(err)
// 	}
// 	defer w.Close()
// 	if err := w.Add(kdir); err != nil {
// 		panic(err)
// 	}
// 	for _, dir := range subdirs(qdir) {
// 		base := filepath.Base(dir)
// 		c.add(filepath.Join(kdir, base+".log"))
// 	}
// 	for _, m := range glob(kdir, "*.log") {
// 		c.add(m)
// 	}
// 	for {
// 		select {
// 		case <-exitCh:
// 			return
// 		case event := <-w.Events:
// 			switch event.Op {
// 			case fsnotify.Create:
// 				if kfile(event.Name) {
// 					fmt.Println(event)
// 					c.add(event.Name)
// 				}
// 			case fsnotify.Remove:
// 				if kfile(event.Name) {
// 					fmt.Println(event)
// 					c.terminated(event.Name)
// 				}
// 			}
// 		case err := <-w.Errors:
// 			fmt.Println(err)
// 		}
// 	}
// }
