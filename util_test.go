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
	"io"
	"testing"
)

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(b []byte) (int, error) {
	return f(b)
}

func Test_Line(t *testing.T) {
	t.Run("buffer with lines", func(t *testing.T) {
		l := newLine()
		l.buf = append(l.buf, []byte("line1\nline2\nline3")...)
		b, err := l.readFrom(nil)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != "line1" {
			t.Fatal("got:", string(b), "want: line1")
		}
		b, err = l.readFrom(nil)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != "line2" {
			t.Fatal("got:", string(b), "want: line2")
		}
		left := l.buf[l.off:]
		if string(left) != "line3" {
			t.Fatal("got:", string(b), "want: line3")
		}
	})
	t.Run("make room for reading", func(t *testing.T) {
		tests := []struct {
			name   string
			unread int
			offset int
			room   int
			cap    int
		}{
			{"unread0-off0", 0, 0, 100, 100},
			{"unread0-off10", 0, 10, 100, 100},
			{"unreadFull", 100, 0, 100, 200},
			{"unread10-begin", 10, 0, 90, 100},
			{"unread10-middle", 10, 10, 80, 100},
			{"unread10-end", 10, 90, 90, 100},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				buf := make([]byte, 100)
				for i := range buf {
					buf[i] = byte(11 + i)
				}
				l := &line{buf[:tt.offset+tt.unread], tt.offset}
				f := func(p []byte) (int, error) {
					if len(p) != tt.room {
						t.Fatal("got:", len(p), "want:", tt.room)
					}
					return 0, io.EOF
				}
				l.readFrom(readerFunc(f))
				for i := 0; i < tt.unread; i++ {
					if got, want := l.buf[l.off+i], byte(tt.offset+11+i); got != want {
						t.Fatal("unread", i, "got", got, "want", want)
					}
				}
				if cap(l.buf) != tt.cap {
					t.Fatal("cap: got", cap(l.buf), "want", tt.cap)
				}
			})
		}
	})

	t.Run("complete", func(t *testing.T) {
		chunks := []string{"ab", "cd\nef", "gh\n", "ijkl\nmnop\n", "qrst\nuvwx\nyz12\n3456"}
		lines := []string{"abcd", "efgh", "ijkl", "mnop", "qrst", "uvwx", "yz12", "3456"}
		f := func(b []byte) (int, error) {
			if len(chunks) == 0 {
				return 0, io.EOF
			}
			chunk := chunks[0]
			chunks = chunks[1:]
			copy(b, chunk)
			if len(chunks) == 0 {
				return len(chunk), io.EOF
			}
			return len(chunk), nil
		}
		l := newLine()
		for len(lines) > 0 {
			want := lines[0]
			lines = lines[1:]
			b, err := l.readFrom(readerFunc(f))
			fmt.Println(string(b), err)
			if err == io.EOF {
				b = l.buffer()
			} else if err != nil {
				t.Fatal(err)
			}
			if string(b) != want {
				t.Fatal("got:", string(b), "want:", want)
			}
		}
	})
}
