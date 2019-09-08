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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/json"
	"github.com/santhosh-tekuri/logflow/kubectl"
)

func getLogFile(dir string, ext int) string {
	return filepath.Join(dir, fmt.Sprintf("log.%d", ext))
}

func extInt(name string) int {
	ext := filepath.Ext(name)
	i, err := strconv.Atoi(ext[1:])
	if err != nil {
		panic(err)
	}
	return i
}

func nextLogFile(name string) string {
	i := extInt(name)
	return filepath.Join(filepath.Dir(name), "log."+strconv.Itoa(i+1))
}

func createMetadataFile(dir string, meta map[string]interface{}) {
	k8s := filepath.Join(dir, ".k8s")
	if !fileExists(k8s) {
		b, err := json.Marshal(meta)
		if err != nil {
			panic(err)
		}
		if err := ioutil.WriteFile(k8s, b, 0700); err != nil {
			panic(err)
		}
	}
}

func markTerminated(dir string) {
	if fileExists(termFile(dir)) {
		return
	}
	logs := getLogFiles(dir)
	if len(logs) == 0 {
		if err := os.RemoveAll(dir); err != nil {
			warn(err)
		}
		return
	}
	if !isEndFile(logs[len(logs)-1]) {
		next := nextLogFile(logs[len(logs)-1])
		if err := ioutil.WriteFile(next, []byte("END\n"), 0700); err != nil {
			panic(err)
		}
		notifyAddFile(dir)
	}
	f, err := os.Create(termFile(dir))
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
	if len(logs) == 1 && fileExists(termFile(dir)) {
		return false
	}
	return true
}

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
	for k, v := range pod.Metadata.Labels {
		nk := strings.ReplaceAll(k, ".", "_")
		if k != nk {
			delete(pod.Metadata.Labels, k)
			pod.Metadata.Labels[nk] = v
		}
	}
	k8s["labels"] = pod.Metadata.Labels
	k8s["nodename"] = pod.Spec.NodeName
	cname := k8s["container_name"].(string)
	if s, ok := pod.Metadata.Annotations["logflow.io/conf_"+cname]; ok {
		k8s["annotation"] = s
	} else if s, ok := pod.Metadata.Annotations["logflow.io/conf"]; ok {
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

func isEndFile(path string) bool {
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

func termFile(dir string) string {
	return filepath.Join(dir, ".terminated")
}

func removeLogFile(dir string) bool {
	files := getLogFiles(dir)
	if len(files) == 0 {
		return false
	}
	if fileExists(termFile(dir)) {
		if len(files) == 1 {
			return false
		}
	} else if len(files) <= maxDockerFiles {
		return false
	}
	f := files[0]
	if err := os.Remove(f); err == nil {
		info(" discard", f[len(qdir):])
		numFilesMu.Lock()
		if _, ok := numFiles[dir]; ok {
			numFiles[dir]--
		}
		numFilesMu.Unlock()
		return true
	} else if !os.IsNotExist(err) {
		panic(err)
	}
	return false
}
