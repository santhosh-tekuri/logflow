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
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/santhosh-tekuri/logflow/kubectl"
)

type pod struct {
	kfile string
	dfile string
	dir   string
}

func (p *pod) save() {
	logs := getLogFiles(p.dir)
	lfile, lext := "", -1
	if len(logs) > 0 {
		lfile = logs[len(logs)-1]
		lext = extInt(lfile)
	}
	if lfile != "" {
		if sameFile(p.dfile, lfile) {
			fmt.Println("same file", p.dfile, lfile)
			return
		}
	}
	fmt.Println("linking", p.dfile)
	if err := os.Link(p.dfile, filepath.Join(p.dir, fmt.Sprintf("log.%d", lext+1))); err != nil {
		panic(err)
	}
}

var namePattern = regexp.MustCompile(`(?P<pod>[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*)_(?P<namespace>[^_]+)_(?P<container>.+)-(?P<docker_id>[a-z0-9]{64})$`)

func (p *pod) fetchMetadata() map[string]interface{} {
	g := namePattern.FindStringSubmatch(filepath.Base(p.dir))
	if len(g) == 0 {
		return nil
	}
	k8s := make(map[string]interface{})
	for i, name := range namePattern.SubexpNames() {
		if name != "" {
			k8s[name] = g[i]
		}
	}
	pod, err := kubectl.GetPod(k8s["namespace"].(string), k8s["pod"].(string))
	if err != nil {
		fmt.Println(err)
		return k8s
	}
	k8s["labels"] = pod.Metadata.Labels
	k8s["nodename"] = pod.Spec.NodeName
	if s, ok := pod.Metadata.Annotations["logflow.io/conf"]; ok {
		k8s["annotation"] = s
	}
	return k8s
}

func getLogFiles(dir string) []string {
	logs := glob(dir, "log.*")
	sort.Slice(logs, func(i, j int) bool {
		x, y := extInt(logs[i]), extInt(logs[j])
		return x < y
	})
	return logs
}
