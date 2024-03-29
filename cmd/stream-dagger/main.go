package main

import (
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/ribasushi/DAGger/internal/constants"
	"github.com/ribasushi/DAGger/internal/dagger"
	"github.com/ribasushi/DAGger/internal/util/profiler"
	"github.com/ribasushi/DAGger/internal/util/stream"
)

func main() {

	inStat, statErr := os.Stdin.Stat()
	if statErr != nil {
		log.Fatalf("unexpected error stat()ing stdIN: %s", statErr)
	}

	// Parse CLI and initialize everything
	// On error it will log.Fatal() on its own
	dgr := dagger.NewFromArgv(os.Args)

	if stream.IsTTY(os.Stdin) {
		fmt.Fprint(
			os.Stderr,
			"------\nYou seem to be feeding data straight from a terminal, an odd choice...\nNevertheless will proceed to read until EOF ( Ctrl+D )\n------\n",
		)
	} else if !inStat.Mode().IsRegular() || inStat.Size() > 16*1024*1024 { // SANCHECK - arbitrary
		// Try optimizations if:
		// - not a reguar file (and not a TTY - exempted above)
		// - regular file larger than a certain size (SANCHECK: somewhat arbitrary)
		// An optimization returns os.ErrInvalid when it can't be applied to the file type
		for _, opt := range stream.ReadOptimizations {
			if err := opt.Action(os.Stdin, inStat); err != nil && err != os.ErrInvalid {
				log.Printf("Failed to apply read optimization hint '%s' to stdIN: %s\n", opt.Name, err)
			}
		}
	}

	var profileStop func()
	// starts profiler if available
	if profiler.StartStop != nil {
		profileStop = profiler.StartStop()
	}
	processErr := dgr.ProcessReader(
		os.Stdin,
		nil,
	)
	dgr.Destroy()
	if profileStop != nil {
		profileStop()
	}
	if processErr != nil {
		log.Fatalf("Unexpected error processing STDIN: %s", processErr)
	}

	if constants.PerformSanityChecks {
		if dagger.CheckGoroutineShutdown {
			// when we get here we should have shut down every goroutine there is
			expectRunning := 1
			if runtime.NumGoroutine() > expectRunning {
				stacks := make([]byte, 4*1024*1024)
				stackLen := runtime.Stack(stacks, true)
				log.Fatalf("\n\nUnexpected amount of goroutines: expected %d but %d goroutines still running\n\n%s\n",
					expectRunning,
					runtime.NumGoroutine(),
					stacks[:stackLen],
				)
			}
		}

		// needed to trigger the zcpstring overallocation guards
		// unless we profiled, in which case we did so there already
		if profileStop == nil {
			runtime.GC() // recommended by https://golang.org/pkg/runtime/pprof/#hdr-Profiling_a_Go_program
			runtime.GC() // recommended harder by @warpfork and @kubuxu :cryingbear:
		}
	}

	dgr.OutputSummary()
}
