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
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/santhosh-tekuri/json"
)

func readConf(r io.Reader) (map[string]string, error) {
	m := make(map[string]string)
	br := bufio.NewReader(r)
	for {
		l, err := br.ReadString('\n')
		if strings.HasSuffix(l, "\n") {
			l = strings.TrimSuffix(l, "\n")
		}
		if strings.TrimSpace(l) != "" && l[0] != '#' {
			eq := strings.IndexByte(l, '=')
			if eq == -1 {
				return nil, err
			}
			m[l[:eq]] = l[eq+1:]
		}
		if err != nil {
			if err != io.EOF {
				return nil, err
			}
			return m, nil
		}
	}
}

func parseSize(s string) (int64, error) {
	if len(s) == 0 {
		return 0, errors.New("invalid size: " + s)
	}
	unit := int64(1)
	switch s[len(s)-1] {
	case 'K':
		unit, s = 1024, s[:len(s)-1]
	case 'M':
		unit, s = 1024*1024, s[:len(s)-1]
	case 'G':
		unit, s = 1024*1024*1024, s[:len(s)-1]
	case 'T':
		unit, s = 1024*1024*1024*1024, s[:len(s)-1]
	case 'P':
		unit, s = 1024*1024*1024*1024*1024, s[:len(s)-1]
	case 'E':
		unit, s = 1024*1024*1024*1024*1024*1024, s[:len(s)-1]
	}
	sz, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, errors.New("invalid size: " + s)
	}
	return sz * unit, nil
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

func extInt(name string) int {
	ext := filepath.Ext(name)
	i, err := strconv.Atoi(ext[1:])
	if err != nil {
		panic(err)
	}
	return i
}

func stat(name string) os.FileInfo {
	fi, err := os.Stat(name)
	if err != nil {
		panic(err)
	}
	return fi
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

func info(a ...interface{}) {
	fmt.Printf("[INFO] ")
	fmt.Println(a...)
}

func warn(a ...interface{}) {
	fmt.Printf("[WARN] ")
	fmt.Println(a...)
}
