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
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/santhosh-tekuri/json"
)

type parser struct {
	dir     string
	records chan<- record
	closed  chan struct{}
	added   chan struct{}
	removed chan struct{}
}

func (p *parser) run() {
	info(" parsing", p.dir[len(qdir):])

	// init ext & pos
	logs := getLogFiles(p.dir)
	ext := extInt(logs[0])
	pos := int64(0)
	b, err := ioutil.ReadFile(filepath.Join(p.dir, ".pos"))
	if err == nil && len(b) == 16 {
		i := byteOrder.Uint64(b)
		j := byteOrder.Uint64(b[8:])
		ext, pos = int(i), int64(j)
	}

	// open file & seek
	f := getLogFile(p.dir, ext)
	for !fileExists(f) {
		f = nextLogFile(f)
		pos = 0
	}
	fnext := nextLogFile(f)
	r, err := os.Open(f)
	if err != nil {
		panic(err)
	}
	resetAdded := func() {
		select {
		case <-p.added:
		default:
		}
		if fileExists(fnext) {
			select {
			case p.added <- struct{}{}:
			default:
			}
		}
	}
	resetAdded()
	if pos != 0 {
		if _, err := r.Seek(pos-1, io.SeekStart); err != nil {
			panic(err)
		}

		// move pos to beginning of line, if it is not the case
		b := make([]byte, 1)
		c := 0
		for {
			n, err := r.Read(b)
			if n == 1 {
				c++
				if b[0] == '\n' {
					break
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				panic(err)
			}
		}
		pos += int64(c - 1)
	}

	// read .k8s
	k8s, err := ioutil.ReadFile(filepath.Join(p.dir, ".k8s"))
	if err != nil {
		b = []byte("{}")
	}
	m, err := jsonUnmarshal(k8s)
	if err != nil {
		panic(err)
	}
	a8n := &annotation{
		de:    json.NewByteDecoder(nil),
		deBuf: make([]byte, 1024),
	}
	if s, ok := m["annotation"]; ok {
		delete(m, "annotation")
		if err := a8n.unmarshal(s.(string)); err != nil {
			warn("error in annotation of", m["pod"], "in", m["namespace"], ":", err)
		}
	}
	k8s, err = json.Marshal(m)
	if err != nil {
		panic(err)
	}

	var rec map[string]interface{}
	sendRec := func() (exit bool) {
		rec["@k8s"] = json.RawMessage(k8s)
	L:
		for {
			select {
			case <-exitCh:
				return true
			case <-p.removed:
				if r != nil && !fileExists(f) {
					_ = r.Close()
					r = nil
				}
			case p.records <- record{
				dir: p.dir,
				ext: ext,
				pos: pos,
				doc: rec,
			}:
				break L
			}
		}
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
	var raw rawLog
	for {
		for r == nil {
			f = nextLogFile(f)
			if fileExists(f) {
				fnext = nextLogFile(f)
				r, err = os.Open(f)
				if err != nil {
					panic(err)
				}
				ext = extInt(f)
				pos = 0
				nl.reset()
				resetAdded()
			}
		}
		l, err := nl.readFrom(r)
		switch err {
		case io.EOF:
			if rec != nil && wait >= multid {
				if exit := sendRec(); exit {
					return
				}
				continue
			}
			select {
			case <-p.added:
				if fileExists(fnext) {
					_ = r.Close()
					r = nil
					continue
				}
			default:
			}
			timer.Reset(d)
			select {
			case <-exitCh:
				return
			case <-p.removed:
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
					case <-p.removed:
						continue
					case <-exitCh:
					case p.records <- record{
						dir: p.dir,
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
				rec["@message"] = rec["@message"].(string) + "\n" + raw.Log
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

var nlSuffix = []byte(`\n"`)

func (r *rawLog) unmarshal(de json.Decoder) error {
	return json.UnmarshalObj("rawLog", de, func(de json.Decoder, prop json.Token) (err error) {
		switch {
		case prop.Eq("time"):
			r.Time, err = de.Token().String("rawLog.Time")
		case prop.Eq("log"):
			t := de.Token()
			if t.Kind == json.Str && bytes.HasSuffix(t.Data, nlSuffix) {
				t.Data = t.Data[:len(t.Data)-2]
			}
			r.Log, err = t.String("rawLog.Log")
		default:
			err = de.Skip()
		}
		return
	})
}
