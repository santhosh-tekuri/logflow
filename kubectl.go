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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"

	"github.com/santhosh-tekuri/json"
)

var (
	kubeClient *http.Client
	base       string
	auth       string
)

func init() {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if len(host) == 0 || len(port) == 0 {
		return
	}
	base = "https://" + net.JoinHostPort(host, port) + "/api/v1"

	b, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		panic(err)
	}
	auth = "Bearer " + string(b)

	b, err = ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	if err != nil {
		panic(err)
	}
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(b)
	kubeClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: certPool,
			},
		},
	}
}

//go:generate jsonc -o kubectl_json.go pod

type pod struct {
	Metadata struct {
		Labels      map[string]interface{} `json:"labels"`
		Annotations map[string]string      `json:"annotations"`
	} `json:"metadata"`
	Spec struct {
		NodeName string `json:"nodeName"`
	} `json:"spec"`
}

var errNonKubernetes = errors.New("non kubernetes environment")

func getPod(ns, podName string) (pod, error) {
	if kubeClient == nil {
		return pod{}, errNonKubernetes
	}
	req, err := http.NewRequest(http.MethodGet, base+"/namespaces/"+ns+"/pods/"+podName, http.NoBody)
	if err != nil {
		panic(err)
	}
	req.Header.Add("Authorization", auth)
	resp, err := kubeClient.Do(req)
	if err != nil {
		return pod{}, err
	}
	defer func() {
		_, _ = io.Copy(os.Stdout, resp.Body)
		_ = resp.Body.Close()
	}()
	switch resp.StatusCode {
	case http.StatusNotFound:
		return pod{}, nil
	case http.StatusOK:
		var p pod
		err = p.DecodeJSON(json.NewReadDecoder(resp.Body))
		return p, err
	default:
		return pod{}, errors.New(resp.Status)
	}
}
