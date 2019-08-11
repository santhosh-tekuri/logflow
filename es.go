package main

import (
	"bytes"
	"fmt"
	"time"
)

func export(r *records) {
	i := 0
	buf := new(bytes.Buffer)
	for {
		rec, err := r.next(time.Second)
		if err == nil {
			i++
			buf.Write(rec.json)
			buf.WriteByte('\n')
		}
		if err == errExit {
			return
		}
		if err == errTimeout || i == 100 {
			r.commit()
			fmt.Println(buf)
			fmt.Println("committed...", err, i)
			i = 0
		}
	}
}
