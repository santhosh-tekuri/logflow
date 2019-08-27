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
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/json"
	"github.com/santhosh-tekuri/logflow/kubectl"
)

func createMetadataFile(dir string) {
	k8s := filepath.Join(dir, ".k8s")
	if !fileExists(k8s) {
		m := fetchMetadata(filepath.Base(dir))
		b, err := json.Marshal(m)
		if err != nil {
			panic(err)
		}
		if err := ioutil.WriteFile(k8s, b, 0700); err != nil {
			panic(err)
		}
	}
}

func markTerminated(dir string) {
	if fileExists(filepath.Join(dir, ".terminated")) {
		return
	}
	logs := getLogFiles(dir)
	if len(logs) == 0 {
		if err := os.RemoveAll(dir); err != nil {
			warn(err)
		}
		return
	}
	if !IsEndFile(logs[len(logs)-1]) {
		next := nextLogFile(logs[len(logs)-1])
		if err := ioutil.WriteFile(next, []byte("END\n"), 0700); err != nil {
			panic(err)
		}
	}
	f, err := os.Create(filepath.Join(dir, ".terminated"))
	if err != nil {
		panic(err)
	}
	_ = f.Close()
}

func hasLogs(dir string) bool {
	logs := getLogFiles(dir)
	if len(logs) == 0 {
		return false
	}
	if len(logs) == 1 && fileExists(filepath.Join(dir, ".terminated")) {
		return false
	}
	return true
}

// options
var (
	a8nName = "logflow.io/conf"
	dotAlt  = "_"
)

func fetchMetadata(logName string) map[string]interface{} {
	k8s := parseLogName(logName)
	if k8s == nil {
		return nil
	}
	pod, err := kubectl.GetPod(k8s["namespace"].(string), k8s["pod"].(string))
	if err != nil {
		warn(err)
		return k8s
	}
	if dotAlt != "" {
		for k, v := range pod.Metadata.Labels {
			nk := strings.ReplaceAll(k, ".", dotAlt[:1])
			if k != nk {
				delete(pod.Metadata.Labels, k)
				pod.Metadata.Labels[nk] = v
			}
		}
	}
	k8s["labels"] = pod.Metadata.Labels
	k8s["nodename"] = pod.Spec.NodeName
	if s, ok := pod.Metadata.Annotations[a8nName]; ok {
		k8s["annotation"] = s
	}
	return k8s
}

func parseLogName(name string) map[string]interface{} {
	i := strings.IndexByte(name, '_')
	if i == -1 {
		return nil
	}
	pod, name := name[:i], name[i+1:]

	i = strings.IndexByte(name, '_')
	if i == -1 {
		return nil
	}
	ns, name := name[:i], name[i+1:]

	i = strings.LastIndexByte(name, '-')
	if i == -1 {
		return nil
	}
	cid, cname := name[i+1:], name[:i]
	return map[string]interface{}{
		"pod":            pod,
		"namespace":      ns,
		"container_name": cname,
		"container_id":   cid,
	}
}

func getLogFiles(dir string) []string {
	logs := glob(dir, "log.*")
	sort.Slice(logs, func(i, j int) bool {
		x, y := extInt(logs[i]), extInt(logs[j])
		return x < y
	})
	return logs
}

func IsEndFile(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		panic(err)
	}
	if fi.Size() != 4 {
		return false
	}
	b, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return bytes.Equal(b, []byte("END\n"))
}
