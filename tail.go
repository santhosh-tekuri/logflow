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
	"strconv"
	"sync"
	"time"
)

// tail watches inode change of log-files and
// creates a hard link in dstDir when inode changes.
type tail struct {
	mu sync.Mutex
	m  map[string]*logRef
}

// follow registers file to detect inode change
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

// stop stops following the logFile
func (t *tail) stop(logFile string) {
	t.mu.Lock()
	delete(t.m, logFile)
	t.mu.Unlock()
}

// run polls for inode changes of logFiles
// periodically and takes action on inode change
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
	files := make([]string, 0, maxDockerFiles)
	if lfile != "" && sameFile(lfile, logFile) {
		return
	}
	files = append(files, logFile)
	for i := 1; true; i++ {
		f := logFile + "." + strconv.Itoa(i)
		if !fileExists(f) {
			break
		}
		if lfile != "" && sameFile(lfile, f) {
			break
		}
		files = append(files, f)
	}
	for len(files) > 0 {
		logFile := files[len(files)-1]
		files = files[:len(files)-1]
		lext++

		dstFile := filepath.Join(lr.dst, fmt.Sprintf("log.%d", lext))
		info(" storing", dstFile[len(qdir):])
		if err := os.Link(logFile, dstFile); err != nil {
			panic(err)
		}
		numFilesMu.Lock()
		numFiles[lr.dst]++
		numFilesMu.Unlock()
		checkMaxFiles()
	}
}

func checkMaxFiles() {
	// check maxFiles
	numFilesMu.Lock()
	c := 0
	rmdir := ""
	for dir, n := range numFiles {
		f := filepath.Join(kdir, filepath.Base(dir)+".log")
		if fileExists(f) {
			if n <= maxDockerFiles {
				continue
			}
			n -= maxDockerFiles
		} else {
			n -= 1 // exclude end file
		}
		if n > 0 {
			c += n
			rmdir = dir
		}
	}
	numFilesMu.Unlock()
	if c > maxFiles {
		if removeLogFile(rmdir) {
			parsersMu.Lock()
			p, ok := parsers[rmdir]
			parsersMu.Unlock()
			if ok {
				select {
				case <-exitCh:
				case p.removed <- struct{}{}:
				case <-p.closed:
				}
			}
		}
	}
}
