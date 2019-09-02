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
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/santhosh-tekuri/json"
)

func parseLogs(dir string, records chan<- record, removed chan struct{}) {
	info(" parsing", dir[len(qdir):])

	// init ext & pos
	logs := getLogFiles(dir)
	ext := extInt(logs[0])
	pos := int64(0)
	b, err := ioutil.ReadFile(filepath.Join(dir, ".pos"))
	if err == nil && len(b) == 16 {
		i := byteOrder.Uint64(b)
		j := byteOrder.Uint64(b[8:])
		ext, pos = int(i), int64(j)
	}

	// open file & seek
	f := getLogFile(dir, ext)
	for !fileExists(f) {
		f = nextLogFile(f)
		pos = 0
	}
	r, err := os.Open(f)
	if err != nil {
		panic(err)
	}
	if pos != 0 {
		if _, err := r.Seek(pos-1, io.SeekStart); err != nil {
			panic(err)
		}

		// move pos to beginning of line, if it is not the case
		b := make([]byte, 1)
		c := 0
		for {
			c++
			n, err := r.Read(b)
			if err != nil {
				if err == io.EOF {
					break
				}
				panic(err)
			}
			if n == 1 {
				if b[0] == '\n' {
					break
				}
			} else {
				time.Sleep(time.Second)
			}
		}
	}

	// read .k8s
	k8s, err := ioutil.ReadFile(filepath.Join(dir, ".k8s"))
	if err != nil {
		b = []byte("{}")
	}
	m, err := jsonUnmarshal(k8s)
	if err != nil {
		panic(err)
	}
	a8n := new(annotation)
	if s, ok := m["annotation"]; ok {
		delete(m, "annotation")
		if err := a8n.unmarshal(s.(string)); err != nil {
			warn(err)
		}
	}
	k8s, err = json.Marshal(m)
	if err != nil {
		panic(err)
	}

	//file := filepath.Base(dir) + ".log"
	var rec map[string]interface{}
	sendRec := func() (exit bool) {
		//rec["@file"] = file
		rec["@k8s"] = json.RawMessage(k8s)
	L:
		for {
			select {
			case <-exitCh:
				return true
			case <-removed:
				if r != nil && !fileExists(f) {
					_ = r.Close()
					r = nil
				}
			case records <- record{
				dir: dir,
				ext: ext,
				pos: pos,
				doc: rec,
			}:
				break L
			}
		}
		//fmt.Printf("%s\n", string(b))
		rec = nil
		return false
	}

	de := json.NewByteDecoder(nil)
	const d = 1 * time.Second
	const multid = 5 * time.Second

	nl := newLine()
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	wait := 0 * time.Second
	for {
		for r == nil {
			info("skipping", f[len(qdir):])
			f = nextLogFile(f)
			if fileExists(f) {
				r, err = os.Open(f)
				if err != nil {
					panic(err)
				}
				ext = extInt(f)
				pos = 0
				nl.reset()
			}
		}
		l, err := nl.readFrom(r)
		var raw rawLog
		switch err {
		case io.EOF:
			if rec != nil && wait >= multid {
				if exit := sendRec(); exit {
					return
				}
				continue
			}
			if next := nextLogFile(f); fileExists(next) {
				f = next
				_ = r.Close()
				r, err = os.Open(f)
				if err != nil {
					panic(err)
				}
				ext++
				pos = 0
				nl.reset()
				continue
			}
			timer.Reset(d)
			select {
			case <-exitCh:
				return
			case <-removed:
				if r != nil && !fileExists(f) {
					_ = r.Close()
					r = nil
				}
				if !timer.Stop() {
					<-timer.C
				}
				continue
			case <-timer.C:
				wait += d
				continue
			}
		case nil:
			wait = 0
			if len(l) == 3 && "END" == string(l) {
				if rec != nil {
					sendRec()
				}
				_ = r.Close()
				for {
					select {
					case <-removed:
						continue
					case <-exitCh:
					case records <- record{
						dir: dir,
						ext: -1,
					}:
					}
					return
				}
			}
			de.Reset(l)
			if err := raw.unmarshal(de); err != nil {
				panic(err)
			}
			if rec != nil && a8n.multi.MatchString(raw.Log) {
				if exit := sendRec(); exit {
					return
				}
			}
			pos += int64(len(l) + 1)
			if rec != nil {
				rec["@msg"] = rec["@msg"].(string) + "\n" + raw.Log
				continue
			}
			rec, err = a8n.parse(raw)
			if err != nil {
				warn(err)
				break
			}
			if a8n.multi == nil {
				if exit := sendRec(); exit {
					return
				}
			}
		default:
			panic(err)
		}
	}
}

// rawLog ---

type rawLog struct {
	Time string `json:"time"`
	Log  string `json:"log"`
}

func (r *rawLog) unmarshal(de json.Decoder) error {
	return json.UnmarshalObj("rawLog", de, func(de json.Decoder, prop json.Token) (err error) {
		switch {
		case prop.Eq("time"):
			r.Time, err = de.Token().String("rawLog.Time")
		case prop.Eq("log"):
			r.Log, err = de.Token().String("rawLog.Log")
			if r.Log != "" && r.Log[len(r.Log)-1] == '\n' {
				r.Log = r.Log[:len(r.Log)-1]
			}
		default:
			err = de.Skip()
		}
		return
	})
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

// ---

func getLogFile(dir string, ext int) string {
	return filepath.Join(dir, fmt.Sprintf("log.%d", ext))
}

func nextLogFile(name string) string {
	i := extInt(name)
	return filepath.Join(filepath.Dir(name), "log."+strconv.Itoa(i+1))
}
