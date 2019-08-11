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
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

var byteOrder = binary.BigEndian

type records struct {
	records chan record
	cursors map[string]*cursor
}

func newRecords() *records {
	return &records{
		records: make(chan record),
		cursors: make(map[string]*cursor),
	}
}

var errExit = errors.New("got exit signal")
var errTimeout = errors.New("timeout")

func (r *records) next(timeout time.Duration) (record, error) {
	for {
		select {
		case <-exitCh:
			return record{}, errExit
		case <-time.After(timeout):
			return record{}, errTimeout
		case rec := <-r.records:
			cur, ok := r.cursors[rec.dir]
			if !ok {
				cur = newCursor(rec.dir)
				r.cursors[rec.dir] = cur
			}
			cur.ext, cur.pos = rec.ext, rec.pos
			if cur.ext == -1 {
				continue
			}
			return rec, nil
		}
	}
}

func (r *records) commit() {
	for _, cur := range r.cursors {
		if finished := cur.commit(); finished {
			delete(r.cursors, cur.dir)
		}
	}
}

type record struct {
	dir  string
	ext  int
	pos  int64
	ts   time.Time
	json []byte
}

type cursor struct {
	dir string
	ext int
	pos int64
	f   *os.File
	b   []byte
}

func newCursor(dir string) *cursor {
	f, err := os.OpenFile(filepath.Join(dir, ".pos"), os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		panic(err)
	}
	b := make([]byte, 16)
	n, _ := io.ReadFull(f, b)
	if n != 16 {
		b = make([]byte, 16)
	}
	cur := &cursor{
		dir: dir,
		f:   f,
		b:   b,
	}
	ext, _ := cur.committed()
	cur.delete(0, ext)
	return cur
}

func (cur *cursor) committed() (ext int, pos int64) {
	i := byteOrder.Uint64(cur.b)
	j := byteOrder.Uint64(cur.b[8:])
	return int(i), int64(j)
}

func (cur *cursor) commit() (finished bool) {
	ext, pos := cur.committed()
	if cur.ext == ext && cur.pos == pos {
		return false
	}
	byteOrder.PutUint64(cur.b, uint64(cur.ext))
	byteOrder.PutUint64(cur.b[8:], uint64(cur.pos))
	if _, err := cur.f.Seek(0, io.SeekStart); err != nil {
		panic(err)
	}
	if _, err := cur.f.Write(cur.b); err != nil {
		panic(err)
	}
	return cur.delete(ext, cur.ext)
}

func (cur *cursor) delete(i, ext int) (finished bool) {
	if ext == -1 {
		cur.f.Close()
		fmt.Println("deleting", cur.dir)
		os.RemoveAll(cur.dir)
		return true
	}
	for i < ext {
		f := filepath.Join(cur.dir, fmt.Sprintf("log.%d", i))
		fmt.Println("deleting", f)
		if err := os.RemoveAll(f); err != nil {
			panic(err)
		}
		i++
	}
	return false
}
