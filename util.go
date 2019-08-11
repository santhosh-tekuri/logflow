package main

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

func mkdirs(dir string) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		panic(err)
	}
}

func glob(dir, pat string) []string {
	m, err := filepath.Glob(filepath.Join(dir, pat))
	if err != nil {
		panic(err)
	}
	return m
}

func isSymLink(name string) bool {
	fi, err := os.Lstat(name)
	if err != nil {
		panic(err)
	}
	return fi.Mode()&os.ModeSymlink != 0
}

func readLinks(name string) string {
	for {
		if !isSymLink(name) {
			return name
		}
		dest, err := os.Readlink(name)
		if err != nil {
			panic(err)
		}
		name = dest
	}
}

func sameFile(name1, name2 string) bool {
	fi1, err := os.Stat(name1)
	if err != nil {
		panic(err)
	}
	fi2, err := os.Stat(name2)
	if err != nil {
		panic(err)
	}
	return os.SameFile(fi1, fi2)
}

func extInt(name string) int {
	ext := filepath.Ext(name)
	i, err := strconv.Atoi(ext[1:])
	if err != nil {
		panic(err)
	}
	return i
}

func inode(name string) uint64 {
	fi, err := os.Stat(name)
	if err != nil {
		panic(err)
	}
	return fi.Sys().(*syscall.Stat_t).Ino
}
