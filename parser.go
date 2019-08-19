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
	gojson "encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/santhosh-tekuri/json"
)

func parseLogs(dir string, records chan<- record) {
	fmt.Println("parsing", dir)

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
	f := filepath.Join(dir, fmt.Sprintf("log.%d", ext))
	r, err := os.Open(f)
	if err != nil {
		panic(err)
	}
	if pos != 0 {
		if _, err := r.Seek(pos, io.SeekStart); err != nil {
			panic(err)
		}
	}

	// read .k8s
	b, err = ioutil.ReadFile(filepath.Join(dir, ".k8s"))
	if err != nil {
		b, err = ioutil.ReadFile(filepath.Join(dir, "k8s"))
		if err != nil {
			b = []byte("{}")
			//panic(err)
		}
	}
	k8s, err := jsonUnmarshal(b)
	if err != nil {
		panic(err)
	}
	a8n := new(annotation)
	if s, ok := k8s["annotation"]; ok {
		delete(k8s, "annotation")
		if err := a8n.unmarshal(s.(string)); err != nil {
			fmt.Println(err)
		}
	}

	//file := filepath.Base(dir) + ".log"
	var rec map[string]interface{}
	var recPos = pos
	sendRec := func() (exit bool) {
		//rec["@file"] = file
		rec["@k8s"] = k8s
		b, err := gojson.Marshal(rec)
		if err != nil {
			panic(err)
		}
		ts, err := time.Parse(time.RFC3339Nano, rec["@timestamp"].(string))
		if err != nil {
			panic(err)
		}
		select {
		case <-exitCh:
			return true
		case records <- record{
			dir:  dir,
			ext:  ext,
			pos:  recPos,
			ts:   ts,
			json: b,
		}:
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
			next := nextLogFile(r.Name())
			if fileExists(next) {
				r.Close()
				r, err = os.Open(next)
				if err != nil {
					panic(err)
				}
				ext++
				pos, recPos = 0, 0
				continue
			}
			// fmt.Println("Zzzzzz")
			timer.Reset(d)
			select {
			case <-exitCh:
				return
			case <-timer.C:
				wait += d
				continue
			}
		case nil:
			pos += int64(len(l) + 1)
			wait = 0
			if len(l) == 3 && "END" == string(l) {
				select {
				case <-exitCh:
					return
				case records <- record{
					dir: dir,
					ext: -1,
				}:
				}
				return
			}
			raw = rawLog{}
			de.Reset(l)
			if err := raw.unmarshal(de); err != nil {
				panic(err)
			}
			if rec != nil {
				if a8n.multi.MatchString(raw.Message) {
					if exit := sendRec(); exit {
						return
					}
				} else {
					rec["@message"] = rec["@message"].(string) + "\n" + raw.Message
					recPos = pos
					continue
				}
			}
			rec, err = a8n.parse(raw)
			if err != nil {
				fmt.Println(err)
				break
			}
			recPos = pos
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
	Timestamp string `json:"time"`
	Message   string `json:"log"`
}

func (r *rawLog) unmarshal(de json.Decoder) error {
	return json.UnmarshalObj("rawLog", de, func(de json.Decoder, prop json.Token) (err error) {
		switch {
		case prop.Eq("time"):
			r.Timestamp, err = de.Token().String("rawLog.Timestamp")
		case prop.Eq("log"):
			r.Message, err = de.Token().String("rawLog.Message")
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

// ---

func nextLogFile(name string) string {
	i := extInt(name)
	return filepath.Join(filepath.Dir(name), "log."+strconv.Itoa(i+1))
}
