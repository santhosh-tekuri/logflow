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
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/santhosh-tekuri/json"
)

// options
var (
	bulkLimit   = 5 * 1024 * 1024
	indexPrefix = "logflow-"
	esURL       = "http://elasticsearch:9200"
	esAuth      = ""
)

func export(r *records) {
	url := esURL + "/_bulk"
	body := bytes.NewBuffer(make([]byte, 0, bulkLimit))
	enc := json.NewEncoder(body)
	for {
		rec, err := r.next(500 * time.Millisecond)
		if err == errExit {
			return
		}
		if err == nil {
			body.WriteString(`{"index":{"_index":"`)
			body.WriteString(indexPrefix)
			ts := rec.doc["@time"].(string)
			body.WriteString(ts[:4])   // year
			body.WriteString(ts[5:7])  // month
			body.WriteString(ts[8:10]) // date
			body.WriteString("\"}}\n")
			if err := enc.Encode(rec.doc); err != nil {
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

var discardBuf = make([]byte, 1024)

func bulk(esurl string, body []byte) error {
	b := body[0:cap(body)]
	for body != nil {
		req, err := http.NewRequest(http.MethodPost, esurl, bytes.NewReader(body))
		if err != nil {
			panic(err)
		}
		req = req.WithContext(exitCtx)
		if esAuth != "" {
			req.Header.Set("Authorization", esAuth)
		}
		req.Header.Set("Content-Type", "application/x-ndjson")
		req.ContentLength = int64(len(body))
		resp, err := esClient.Do(req)
		if err != nil {
			if uerr, ok := err.(*url.Error); ok && uerr.Err == context.Canceled {
				return uerr.Err
			}
			return err
		}
		if resp.StatusCode > 299 {
			warn("elasticsearch returned", resp.Status)
			body = nil
		} else {
			ss, err := bulkSuccessful(resp.Body)
			if err != nil {
				warn("bulkResponse.decode:", err)
				body = nil
			} else {
				body = checkIndexErrors(body, ss)
			}
		}
		buf := b
		if body != nil {
			buf = discardBuf
		}
		_, _ = io.CopyBuffer(ioutil.Discard, resp.Body, buf)
		return resp.Body.Close()
	}
	return nil
}

var (
	errStop     = errors.New("stop unmarshalling")
	bulkDecoder = json.NewReadDecoder(nil)
)

func bulkSuccessful(r io.Reader) ([]int, error) {
	bulkDecoder.Reset(r)
	var idx []int
	err := json.UnmarshalObj("bulk", bulkDecoder, func(d json.Decoder, prop json.Token) (err error) {
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
	bulkDecoder.Reset(nil)
	return idx, err
}

func checkIndexErrors(body []byte, success []int) []byte {
	i := 0
	for i < len(success) {
		if success[i] == -1 {
			panic("success==-1")
		} else if success[i] == 0 {
			break
		}
		i++
	}
	if i == len(success) {
		return nil
	}

	from := 0
	for j := 0; j < i; j++ {
		from += bytes.IndexByte(body[from:], '\n') + 1
		from += bytes.IndexByte(body[from:], '\n') + 1
	}
	to := from + bytes.IndexByte(body[from:], '\n') + 1
	to += bytes.IndexByte(body[to:], '\n') + 1
	i++

	for x := to; i < len(success); i++ {
		y := x + bytes.IndexByte(body[x:], '\n') + 1
		y += bytes.IndexByte(body[y:], '\n') + 1
		if success[i] == -1 {
			panic("success==-1")
		} else if success[i] == 0 {
			if x == to {
				to = y
			} else {
				copy(body[to:], body[x:y])
				to += y - x
			}
		}
		x = y
	}
	return body[from:to]
}

var esClient = &http.Client{
	Transport: &http.Transport{
		Dial: (&net.Dialer{
			Timeout:   20 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	},
}

func parseExportConf(m map[string]string) error {
	s, ok := m["elasticsearch.url"]
	if !ok {
		return errors.New("config: elasticsearch.url missing")
	}
	esURL = s
	if s, ok = m["elasticsearch.cacert"]; ok {
		b, err := ioutil.ReadFile(s)
		if err != nil {
			return err
		}
		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(b)
		t := esClient.Transport.(*http.Transport).TLSClientConfig
		t.InsecureSkipVerify = false
		t.RootCAs = certPool
	}
	if s, ok = m["elasticsearch.clientcert"]; ok {
		key, ok := m["elasticsearch.clientkey"]
		if !ok {
			return errors.New("config: elasticsearch.clientkey missing")
		}
		clientCert, err := tls.LoadX509KeyPair(s, key)
		if err != nil {
			return err
		}
		t := esClient.Transport.(*http.Transport).TLSClientConfig
		t.Certificates = []tls.Certificate{clientCert}
	}
	if s, ok = m["elasticsearch.basicAuth"]; ok {
		if strings.IndexByte(s, ':') == -1 {
			return errors.New("config: elasticsearch.basicAuth has invalid value")
		}
		esAuth = "Basic " + base64.StdEncoding.EncodeToString([]byte(s))
	}
	if s, ok = m["elasticsearch.bulk_size"]; ok {
		sz, err := parseSize(s)
		if err != nil {
			return err
		}
		bulkLimit = int(sz)
	}
	if s, ok = m["elasticsearch.index_name.prefix"]; ok {
		indexPrefix = s
	}
	return nil
}
