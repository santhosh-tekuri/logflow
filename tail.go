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
	"sync"
	"time"
)

type tail struct {
	mu sync.Mutex
	m  map[string]*logRef
}

func (t *tail) follow(logFile, dstDir string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	fi, err := os.Stat(logFile)
	if err != nil {
		panic(err)
	}
	lr := &logRef{
		dst: dstDir,
		fi:  fi,
	}
	lr.save(logFile)
	t.m[logFile] = lr
}

func (t *tail) stop(logFile string) {
	t.mu.Lock()
	delete(t.m, logFile)
	t.mu.Unlock()
}

func (t *tail) run() {
	const d = 250 * time.Millisecond
	timer := time.NewTimer(d)
	for {
		select {
		case <-exitCh:
			return
		case <-timer.C:
			t.mu.Lock()
			for logFile, lr := range t.m {
				fi, err := os.Stat(logFile)
				if err != nil {
					warn(err)
					continue
				}
				if !os.SameFile(fi, lr.fi) {
					lr.fi = fi
					lr.save(logFile)
				}
			}
			t.mu.Unlock()
			timer.Reset(d)
		}
	}
}

// ---

type logRef struct {
	dst string
	fi  os.FileInfo
}

func (lr *logRef) save(logFile string) {
	logs := getLogFiles(lr.dst)
	lfile, lext := "", -1
	if len(logs) > 0 {
		lfile = logs[len(logs)-1]
		lext = extInt(lfile)
	}
	if lfile != "" {
		if sameFile(lfile, logFile) {
			return
		}
	}
	info("storing", logFile)
	dstFile := filepath.Join(lr.dst, fmt.Sprintf("log.%d", lext+1))
	if err := os.Link(logFile, dstFile); err != nil {
		panic(err)
	}
}
