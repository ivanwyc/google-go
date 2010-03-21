package main

// Need to compile package gob with debug.go to build this program.

import (
	"fmt"
	"gob"
	"os"
)

func main() {
	var err os.Error
	file := os.Stdin
	if len(os.Args) > 1 {
		file, err = os.Open(os.Args[1], os.O_RDONLY, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "dump: %s\n", err)
			os.Exit(1)
		}
	}
	gob.Debug(file)
}
