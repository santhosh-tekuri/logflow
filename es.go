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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"time"
)

// options
var (
	bulkLimit   = 2 * 1024 * 1024
	indexLayout = "logflow-20060102"
	esURL       = "http://elasticsearch:9200"
)

func export(r *records) {
	url := esURL + "/_bulk"
	body := new(bytes.Buffer)
	for {
		rec, err := r.next(500 * time.Millisecond)
		if err == errExit {
			return
		}
		if err == nil {
			body.WriteString(`{"index":{"_index":"`)
			body.WriteString(rec.ts.Format(indexLayout))
			body.WriteString("\"}}\n")
			body.Write(rec.json)
			body.WriteByte('\n')
		}
		if body.Len() > 0 && (err == errTimeout || body.Len() >= bulkLimit) {
			if cancelled := bulkRetry(url, body.Bytes()); cancelled {
				return
			}
			r.commit()
			body.Reset()
		}
	}
}

// api call ---

func bulkRetry(url string, body []byte) (cancelled bool) {
	round := 0
	for {
		err := bulk(url, body)
		if err != nil {
			if err == context.Canceled {
				return
			}
			if round == 0 {
				warn(err)
			}
			round++
			select {
			case <-exitCh:
				return true
			case <-time.After(backOff(round, 5*time.Second)):
				continue
			}
		}
		if round > 0 {
			info("elasticsearch is reachable")
		}
		return false
	}
}

func bulk(esurl string, body []byte) error {
	req, err := http.NewRequest(http.MethodPost, esurl, bytes.NewReader(body))
	if err != nil {
		panic(err)
	}
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/x-ndjson")
	req.ContentLength = int64(len(body))
	go func() {
		select {
		case <-ctx.Done():
		case <-exitCh:
			cancel()
		}
	}()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if uerr, ok := err.(*url.Error); ok && uerr.Err == context.Canceled {
			return uerr.Err
		}
		return err
	}
	defer func() {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()
	if resp.StatusCode > 299 {
		io.Copy(os.Stdout, resp.Body)
		fmt.Println()
		warn("elasticsearch returned", resp.Status)
	} else {
		ss, err := bulkSuccessful(bytes.NewReader(body))
		if err != nil {
			warn("bulkResponse.decode:", err)
		} else {
			for i, s := range ss {
				if s == 0 {
					warn("log", i, "unsuccessfull")
				} else if s == -1 {
					panic("success==-1")
				}
			}
		}
	}
	return nil
}

func bulkSuccessful(r io.Reader) ([]int, error) {
	var successful []int
	var stack []json.Token
	peek := func() json.Token {
		if len(stack) == 0 {
			return nil
		}
		return stack[len(stack)-1]
	}
	d := json.NewDecoder(r)
	for {
		t, err := d.Token()
		if err != nil {
			if err == io.EOF {
				return successful, nil
			}
			return nil, err
		}
		switch t {
		case json.Delim('{'), json.Delim('['):
			if len(stack) == 7 && peek() == "error" {
				successful = append(successful, -1)
			}
			stack = append(stack, t)
		case json.Delim(']'), json.Delim('}'):
			stack = stack[:len(stack)-1]
			if _, ok := peek().(string); ok {
				stack = stack[:len(stack)-1]
			}
		default:
			if peek() == json.Delim('{') {
				stack = append(stack, t)
			} else {
				if len(stack) == 9 && peek() == "successful" {
					successful = append(successful, int(t.(float64)))
				}
				stack = stack[:len(stack)-1]
			}
		}
	}
}
