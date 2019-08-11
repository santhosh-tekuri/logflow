package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type pod struct {
	kfile string
	dfile string
	dir   string
}

func (p *pod) save() {
	logs := glob(p.dir, "log.*")
	sort.Slice(logs, func(i, j int) bool {
		x, y := extInt(logs[i]), extInt(logs[j])
		return x < y
	})
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
