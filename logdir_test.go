package main

import (
	"fmt"
	"testing"
)

func TestNamePattern(t *testing.T) {
	tests := []string{
		"kube-flannel-ds-amd64-q9bcs_kube-system_install-cni-bbd6373080f1be86c6f419580d45f4f3b259ef3a98890091a67eaf6abba225ae",
		"speaker-zxw2w_metallb-system_speaker-3c6a6aa7698368490c11a48dd57d9894e2e5a8c5bf4e634fcdf2c61bb939927c",
	}
	for _, tt := range tests {
		k8s := parseLogName(tt)
		if k8s == nil {
			t.Fatalf("regex not matched: %s", tt)
		}
		got := fmt.Sprintf("%s_%s_%s-%s", k8s["pod"], k8s["namespace"], k8s["container_name"], k8s["container_id"])
		if got != tt {
			t.Log(" got:", got)
			t.Log("want:", tt)
			t.Fatal("did not match")
		}
	}
}
