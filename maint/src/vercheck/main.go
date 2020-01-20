package main

import (
	"log"
	"regexp"
	"runtime"
)

func main() {
	re := regexp.MustCompile(`go1\.(?:1[1-9]|[2-9][0-9])\.`)
	if !re.MatchString(runtime.Version()) {
		log.Fatalf(
			"\n\nUnable to continue: Golang version 1.11+ required but this is %s\n\n",
			runtime.Version(),
		)
	}
}
