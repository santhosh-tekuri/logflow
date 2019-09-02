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
)

type annotation struct {
	format   interface{} // "json" or *regexp.Regexp
	tsKey    string
	tsLayout string
	msgKey   string
	multi    *regexp.Regexp
}

func (a8n *annotation) parse(raw rawLog) (map[string]interface{}, error) {
	msg, ts := raw.Log, raw.Time
	rec := make(map[string]interface{})
	switch {
	case a8n.format == nil:
		if len(msg) >= 2 && msg[0] == '{' && msg[len(msg)-1] == '}' {
			m, err := jsonUnmarshal([]byte(msg))
			if err != nil {
				break
			}
			for k, v := range m {
				if k == "msg" || k == "message" {
					msg = fmt.Sprint(v)
				} else {
					if k == "time" || k == "timestamp" || k == "ts" {
						sv := fmt.Sprint(v)
						if _, err := time.Parse(time.RFC3339Nano, sv); err == nil {
							ts = sv
							continue
						}
					}
					rec[k] = v
				}
			}
		}
	case a8n.format == "json":
		m, err := jsonUnmarshal([]byte(msg))
		if err != nil {
			break
		}
		for k, v := range m {
			if k == a8n.msgKey {
				msg = fmt.Sprint(v)
			} else {
				if k == a8n.tsKey {
					if t, err := time.Parse(a8n.tsLayout, fmt.Sprint(v)); err == nil {
						ts = t.Format(time.RFC3339Nano)
						continue
					}
				}
				rec[k] = v
			}
		}
	default:
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

	rec["@msg"] = msg
	rec["@time"] = ts
	return rec, nil
}

func (a8n *annotation) unmarshal(s string) error {
	m, err := readConf(strings.NewReader(s))
	if err != nil {
		return err
	}
	if s, ok := m["multiline_start"]; ok {
		re, err := compileRegex(s)
		if err != nil {
			return err
		}
		a8n.multi = re
	}

	s, ok := m["format"]
	if !ok {
		return nil
	}
	a8n.tsKey = m["time_key"]
	a8n.tsLayout = m["time_layout"]
	if a8n.tsKey != "" && a8n.tsLayout == "" {
		return errors.New("time_layout missing")
	}
	a8n.msgKey = m["msg_key"]
	if a8n.msgKey == "" {
		return errors.New("msg_key missing")
	}
	if s == "json" {
		a8n.format = "json"
		a8n.multi = nil
	} else {
		re, err := compileRegex(s)
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
				return errors.New("time_key missing in regex")
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
			return errors.New("msg_key missing in regex")
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
