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
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

var (
	numFiles   = make(map[string]int)
	numFilesMu = sync.Mutex{}
)

// options
var (
	maxDockerFiles = 3
	maxFiles       = 10
)

func watchContainers(kdir, qdir string, tail *tail, records chan<- record) {
	mkdirs(qdir)

	w, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}
	defer w.Close()
	if err := w.Add(kdir); err != nil {
		panic(err)
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	logDirs := make(map[string]string)

	newContainer := func(logFile string) {
		logDir, logFile := newContainer(logFile, qdir)
		logDirs[logDir] = logFile
		n := len(getLogFiles(logDir))
		numFilesMu.Lock()
		numFiles[logDir] = n
		numFilesMu.Unlock()
		tail.follow(logFile, logDir)
		runParser(&wg, logDir, records)
	}
	for _, logFile := range glob(kdir, "*.log") {
		newContainer(logFile)
	}

	for _, logDir := range subdirs(qdir) {
		if _, ok := logDirs[logDir]; ok {
			continue
		}
		markTerminated(logDir)
		if hasLogs(logDir) {
			n := len(getLogFiles(logDir))
			numFilesMu.Lock()
			numFiles[logDir] = n - 1 // exclude end file
			numFilesMu.Unlock()
			runParser(&wg, logDir, records)
		} else {
			if err := os.RemoveAll(logDir); err != nil {
				warn(err)
			}
		}
	}

	for {
		select {
		case <-exitCh:
			return
		case event := <-w.Events:
			switch event.Op {
			case fsnotify.Create:
				if strings.HasSuffix(event.Name, ".log") {
					newContainer(event.Name)
				}
			case fsnotify.Remove:
				if strings.HasSuffix(event.Name, ".log") {
					id := strings.TrimSuffix(filepath.Base(event.Name), ".log")
					logDir := filepath.Join(qdir, id)
					markTerminated(logDir)
					tail.stop(logDirs[logDir])
					delete(logDirs, logDir)
				}
			}
		case err := <-w.Errors:
			fmt.Println(err)
		}
	}
}

func newContainer(logFile, dstDir string) (logDir, actualLogFile string) {
	id := strings.TrimSuffix(filepath.Base(logFile), ".log")
	logDir = filepath.Join(dstDir, id)
	mkdirs(logDir)
	createMetadataFile(logDir)

	logFile = readLinks(logFile)
	return logDir, logFile
}

type parser struct {
	closed  chan struct{}
	added   chan struct{}
	removed chan struct{}
}

var (
	parsers   = make(map[string]parser)
	parsersMu = sync.Mutex{}
)

func runParser(wg *sync.WaitGroup, dir string, records chan<- record) {
	wg.Add(1)
	go func() {
		p := parser{make(chan struct{}), make(chan struct{}, 1), make(chan struct{})}
		defer func() {
			info("finished", dir[len(qdir):])
			parsersMu.Lock()
			close(p.closed)
			delete(parsers, dir)
			parsersMu.Unlock()
			wg.Done()
		}()
		parsersMu.Lock()
		parsers[dir] = p
		parsersMu.Unlock()
		defer parseLogs(dir, records, p.added, p.removed)
	}()
}
