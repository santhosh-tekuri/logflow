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
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/santhosh-tekuri/json"
)

func readConf(r io.Reader) (map[string]string, error) {
	m := make(map[string]string)
	br := bufio.NewReader(r)
	for {
		l, err := br.ReadString('\n')
		l = strings.TrimSpace(l)
		if l != "" && l[0] != '#' {
			eq := strings.IndexByte(l, '=')
			if eq == -1 {
				return nil, err
			}
			m[strings.TrimSpace(l[:eq])] = strings.TrimSpace(l[eq+1:])
		}
		if err != nil {
			if err != io.EOF {
				return nil, err
			}
			return m, nil
		}
	}
}

func sprint(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// line ---

type line struct {
	buf []byte
	off int
}

func newLine() *line {
	return &line{buf: make([]byte, 0, 8*1024)}
}

func (l *line) readFrom(r io.Reader) ([]byte, error) {
	// check if buffer already has line
	if x := bytes.IndexByte(l.buf[l.off:], '\n'); x != -1 {
		x += l.off
		x, l.off = l.off, x+1
		return l.buf[x : l.off-1], nil
	}

	for {
		// make room of reading
		unread := len(l.buf) - l.off
		if unread == 0 {
			l.buf, l.off = l.buf[0:0], 0
		} else if len(l.buf) == cap(l.buf) {
			if l.off > 0 {
				copy(l.buf[0:], l.buf[l.off:])
			} else {
				b := make([]byte, 2*cap(l.buf))
				copy(b, l.buf[l.off:])
				l.buf = b
			}
			l.buf, l.off = l.buf[:unread], 0
		}

		// read more and check for line
		i := len(l.buf)
		n, err := r.Read(l.buf[i:cap(l.buf)])
		l.buf = l.buf[:i+n]
		if x := bytes.IndexByte(l.buf[i:], '\n'); x != -1 {
			x += i
			x, l.off = l.off, x+1
			return l.buf[x : l.off-1], nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func (l *line) buffer() []byte {
	return l.buf[l.off:]
}

func (l *line) reset() {
	l.buf, l.off = l.buf[0:0], 0
}

// backOff ---

const (
	maxFailureScale = 12
	failureWait     = 10 * time.Millisecond
)

// backOff is used to compute an exponential backOff
// duration. Base time is scaled by the current round,
// up to some maximum scale factor. If backOff exceeds
// given max, it returns max
func backOff(round int, max time.Duration) time.Duration {
	wait := failureWait
	if round > maxFailureScale {
		round = maxFailureScale
	}
	for round > 2 {
		wait *= 2
		round--
	}
	if wait > max {
		return max
	}
	return wait
}

func mkdirs(dir string) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		panic(err)
	}
}

func subdirs(dir string) []string {
	ff, err := ioutil.ReadDir(dir)
	if err != nil {
		panic(err)
	}
	var subdirs []string
	for _, f := range ff {
		if f.IsDir() {
			subdirs = append(subdirs, filepath.Join(dir, f.Name()))
		}
	}
	return subdirs
}

func glob(dir, pat string) []string {
	m, err := filepath.Glob(filepath.Join(dir, pat))
	if err != nil {
		panic(err)
	}
	return m
}

func isSymLink(name string) bool {
	fi, err := os.Lstat(name)
	if err != nil {
		panic(err)
	}
	return fi.Mode()&os.ModeSymlink != 0
}

func readLinks(name string) string {
	for {
		if !isSymLink(name) {
			return name
		}
		dest, err := os.Readlink(name)
		if err != nil {
			panic(err)
		}
		name = dest
	}
}

func sameFile(name1, name2 string) bool {
	fi1, err := os.Stat(name1)
	if err != nil {
		panic(err)
	}
	fi2, err := os.Stat(name2)
	if err != nil {
		panic(err)
	}
	return os.SameFile(fi1, fi2)
}

func fileExists(name string) bool {
	_, err := os.Stat(name)
	if err != nil {
		return !os.IsNotExist(err)
	}
	return true
}

func jsonUnmarshal(line []byte) (map[string]interface{}, error) {
	m, err := json.NewByteDecoder(line).Unmarshal()
	return m.(map[string]interface{}), err
}

// logging ---

var logMu sync.Mutex

func info(a ...interface{}) {
	logMu.Lock()
	fmt.Printf("[INFO] ")
	fmt.Println(a...)
	logMu.Unlock()
}

func warn(a ...interface{}) {
	logMu.Lock()
	fmt.Printf("[WARN] ")
	fmt.Println(a...)
	logMu.Unlock()
}
