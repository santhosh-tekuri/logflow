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
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
)

const (
	kdir = "/var/log/containers/"
	qdir = "/var/log/containers/logflow/"
)

var (
	exitCh  = make(chan struct{})
	exitCtx = context.Background()
)

func main() {
	confFile := "/etc/logflow/logflow.conf"
	if len(os.Args) > 1 {
		confFile = os.Args[1]
	}
	if err := parseConf(confFile); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var wg sync.WaitGroup

	tail := &tail{m: make(map[string]*logRef)}
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer info("tailing exited")
		tail.run()
	}()

	r := newRecords()
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer info("exporter exited")
		export(r)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		watchContainers(kdir, qdir, tail, r.records)
	}()

	var cancel context.CancelFunc
	exitCtx, cancel = context.WithCancel(context.Background())
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch
	close(exitCh)
	cancel()
	wg.Wait()
}

func parseConf(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	m, err := readConf(f)
	if err != nil {
		return err
	}
	s, ok := m["json-file.max-file"]
	if !ok {
		return errors.New("config: json-file.max-file missing")
	}
	maxDockerFiles, err = strconv.Atoi(s)
	if err != nil {
		return err
	}
	if s, ok := m["maxFiles"]; ok {
		maxFiles, err = strconv.Atoi(s)
		if err != nil {
			return err
		}
	}
	if s, ok := m["annotation"]; ok {
		a8nName = s
	}
	if s, ok := m["dot_replacer"]; ok {
		dotAlt = s
	}
	return parseExportConf(m)
}
