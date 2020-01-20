package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/ribasushi/DAGger/internal/repacker"
	"github.com/ribasushi/DAGger/internal/util/stream"
)

func main() {

	if stream.IsTTY(os.Stdout) {
		log.Fatal("Streaming to a TTY is not supported")
	}

	if s, err := os.Stdout.Stat(); err != nil {
		log.Printf("Failed to stat() stdOUT: %s", err)
	} else {
		for _, opt := range stream.WriteOptimizations {
			if err := opt.Action(os.Stdout, s); err != nil && err != os.ErrInvalid {
				log.Printf("Failed to apply write optimization hint '%s' to stdOUT: %s\n", opt.Name, err)
			}
		}
	}

	rpk, filenames := repacker.NewFromArgs(os.Args)

	pts := make([]repacker.PathTuple, 0, len(filenames))
	for _, name := range filenames {
		abs, err := filepath.Abs(name)
		if err != nil {
			log.Fatalf("Determining absolute name of argument '%s' failed: %s", name, err)
		}

		lstat, err := os.Lstat(abs)
		if err != nil {
			log.Fatalf("lstat() of argument '%s' failed: %s", name, err)
		}

		pts = append(pts, repacker.PathTuple{
			Path:  abs,
			Lstat: lstat,
		})
	}

	if err := rpk.RecursePaths(pts); err != nil {
		log.Fatal(err)
	}
}
