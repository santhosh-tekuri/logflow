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
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/santhosh-tekuri/json"
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
			ts, err := time.Parse(time.RFC3339Nano, rec.doc["@timestamp"].(string))
			if err != nil {
				panic(err)
			}
			body.WriteString(`{"index":{"_index":"`)
			body.WriteString(ts.Format(indexLayout))
			body.WriteString("\"}}\n")
			if err := json.NewEncoder(body).Encode(rec.doc); err != nil {
				panic(err)
			}
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

var discardBuf = make([]byte, 100)

func bulk(esurl string, body []byte) error {
	req, err := http.NewRequest(http.MethodPost, esurl, bytes.NewReader(body))
	if err != nil {
		panic(err)
	}
	ctx, cancel := context.WithCancel(req.Context())
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
	cancel()
	if err != nil {
		if uerr, ok := err.(*url.Error); ok && uerr.Err == context.Canceled {
			return uerr.Err
		}
		return err
	}
	if resp.StatusCode > 299 {
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
	_, _ = io.CopyBuffer(ioutil.Discard, resp.Body, discardBuf)
	return resp.Body.Close()
}

var errStop = errors.New("stop unmarshalling")

func bulkSuccessful(r io.Reader) ([]int, error) {
	d := json.NewReadDecoder(r)
	var idx []int
	err := json.UnmarshalObj("bulk", d, func(d json.Decoder, prop json.Token) (err error) {
		switch {
		case prop.Eq("errors"):
			var errors bool
			errors, err = d.Token().Bool("bulk.errors")
			if !errors {
				return errStop
			}
		case prop.Eq("items"):
			err = json.UnmarshalArr("items", d, func(d json.Decoder) error {
				return json.UnmarshalObj("items[]", d, func(d json.Decoder, prop json.Token) (err error) {
					return json.UnmarshalObj("index", d, func(d json.Decoder, prop json.Token) (err error) {
						switch {
						case prop.Eq("error"):
							idx = append(idx, -1)
							err = d.Skip()
						case prop.Eq("_shards"):
							return json.UnmarshalObj("_shards", d, func(d json.Decoder, prop json.Token) (err error) {
								switch {
								case prop.Eq("successful"):
									var i int
									i, err = d.Token().Int("successful")
									idx = append(idx, i)
								default:
									err = d.Skip()
								}
								return err
							})
						default:
							err = d.Skip()
						}
						return
					})
				})
			})
		default:
			err = d.Skip()
		}
		return
	})
	if err == errStop {
		err = nil
	}
	return idx, err
}

func parseExportConf(m map[string]string) error {
	s, ok := m["elasticsearch.url"]
	if !ok {
		return errors.New("config: elasticsearch.url missing")
	}
	esURL = s
	if s, ok = m["elasticsearch.bulk_size"]; ok {
		sz, err := parseSize(s)
		if err != nil {
			return err
		}
		bulkLimit = int(sz)
	}
	if s, ok = m["elasticsearch.index_name.layout"]; ok {
		indexLayout = s
	}
	return nil
}
