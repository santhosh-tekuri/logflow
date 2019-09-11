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
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/santhosh-tekuri/json"
)

type annotation struct {
	format   interface{} // "json" or *regexp.Regexp
	tsKey    string
	tsLayout string
	msgKey   string
	multi    *regexp.Regexp
	de       *json.ByteDecoder
	deBuf    []byte
}

func (a8n *annotation) parse(raw rawLog) (map[string]interface{}, error) {
	msg, ts := raw.Log, raw.Time
	var rec map[string]interface{}
	var err error
	switch {
	case a8n.format == nil:
		if len(msg) >= 2 && msg[0] == '{' && msg[len(msg)-1] == '}' {
			rec, err = a8n.jsonUnmarshal(msg)
			if err != nil {
				break
			}
			for k, v := range rec {
				if k == "msg" || k == "message" {
					msg = fmt.Sprint(v)
					delete(rec, k)
				} else {
					if k == "time" || k == "timestamp" || k == "ts" {
						sv := fmt.Sprint(v)
						if _, err := time.Parse(time.RFC3339Nano, sv); err == nil {
							ts = sv
							delete(rec, k)
							continue
						}
					}
					var suffix string
					switch v.(type) {
					case string:
						rec[k] = v
					case float64:
						suffix = "$num"
					case bool:
						suffix = "$bool"
					case map[string]interface{}:
						suffix = "$obj"
					case []interface{}:
						suffix = "$arr"
					}
					if suffix != "" && !strings.HasSuffix(k, suffix) {
						delete(rec, k)
						rec[k+suffix] = v
					}
				}
			}
		}
	case a8n.format == "json":
		rec, err = a8n.jsonUnmarshal(msg)
		if err != nil {
			break
		}
		for k, v := range rec {
			if k == a8n.msgKey {
				msg = fmt.Sprint(v)
				delete(rec, k)
			} else {
				if k == a8n.tsKey {
					if t, err := time.Parse(a8n.tsLayout, fmt.Sprint(v)); err == nil {
						ts = t.Format(time.RFC3339Nano)
						delete(rec, k)
						continue
					}
				}
				var suffix string
				switch v.(type) {
				case string:
					rec[k] = v
				case float64:
					suffix = "$num"
				case bool:
					suffix = "$bool"
				case map[string]interface{}:
					suffix = "$obj"
				case []interface{}:
					suffix = "$arr"
				}
				if suffix != "" && !strings.HasSuffix(k, suffix) {
					delete(rec, k)
					rec[k+suffix] = v
				}
			}
		}
	default:
		rec = make(map[string]interface{})
		re := a8n.format.(*regexp.Regexp)
		g := re.FindStringSubmatch(msg)
		if len(g) == 0 {
			break
		}
		for i, name := range re.SubexpNames() {
			switch name {
			case "":
				continue
			case a8n.msgKey:
				msg = g[i]
			case a8n.tsKey:
				if t, err := time.Parse(a8n.tsLayout, g[i]); err == nil {
					ts = t.Format(time.RFC3339Nano)
				} else {
					rec[name] = g[i]
				}
			default:
				rec[name] = g[i]
			}
		}
	}

	if rec == nil {
		rec = make(map[string]interface{})
	}
	rec["@message"] = msg
	rec["@timestamp"] = ts
	return rec, nil
}

var errNotMap = errors.New("not map")

func (a8n *annotation) jsonUnmarshal(msg string) (map[string]interface{}, error) {
	a8n.deBuf = append(a8n.deBuf[:0], msg...)
	a8n.de.Reset(a8n.deBuf)
	m, err := a8n.de.Unmarshal()
	if err != nil {
		return nil, err
	}
	if m, ok := m.(map[string]interface{}); ok {
		return m, nil
	}
	return nil, errNotMap
}

func (a8n *annotation) unmarshal(format string) error {
	m, err := readConf(strings.NewReader(format))
	if err != nil {
		return err
	}
	format, ok := m["format"]
	if !ok {
		return nil
	}
	a8n.tsKey = m["timestamp_key"]
	a8n.tsLayout = m["timestamp_layout"]
	if a8n.tsKey != "" && a8n.tsLayout == "" {
		return errors.New("timestamp_layout missing")
	}
	a8n.msgKey = m["message_key"]
	if a8n.msgKey == "" {
		return errors.New("message_key missing")
	}
	if format == "json" {
		a8n.format = "json"
		a8n.multi = nil
	} else {
		re, err := compileRegex(format)
		if err != nil {
			return err
		}
		a8n.format = re

		if a8n.tsKey != "" {
			found := false
			for _, n := range re.SubexpNames() {
				if n == a8n.tsKey {
					found = true
					break
				}
			}
			if !found {
				return errors.New("timestamp_key missing in regex")
			}
		}

		found := false
		for _, n := range re.SubexpNames() {
			if n == a8n.msgKey {
				found = true
				break
			}
		}
		if !found {
			return errors.New("message_key missing in regex")
		}

		if s, ok := m["multiline_start"]; ok {
			re, err := compileRegex(s)
			if err != nil {
				return err
			}
			a8n.multi = re
		}
	}

	return nil
}

func compileRegex(s string) (*regexp.Regexp, error) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '/' || s[len(s)-1] != '/' {
		return nil, errors.New("regex must be enclosed with '/'")
	}
	return regexp.Compile(s[1 : len(s)-1])
}
