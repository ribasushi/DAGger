package repacker

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/pborman/getopt/v2"
	"github.com/pborman/options"
)

type config struct {
	optSet *getopt.Set

	SortDirs bool `getopt:"--sort-dir-contents           When recursing into directories sort their contents just like ioutil.ReadDir would. Default: [false]"`
	Help     bool `getopt:"-h --help                     Display help"`
}

func NewFromArgs(argv []string) (rpk *Repacker, fnArgs []string) {

	rpk = &Repacker{
		cfg: config{
			optSet: getopt.New(),
		},
	}

	cfg := &rpk.cfg

	if err := options.RegisterSet("", cfg, cfg.optSet); err != nil {
		log.Fatalf("option set registration failed: %s", err)
	}
	cfg.optSet.SetParameters("{{list-of-files-or-directories}}\n")

	var argParseErrors []string
	if err := cfg.optSet.Getopt(os.Args, nil); err != nil {
		argParseErrors = append(argParseErrors, err.Error())
	}

	fnArgs = cfg.optSet.Args()
	if len(fnArgs) == 0 {
		argParseErrors = append(argParseErrors, "you must supply one or more paths as arguments")
	}

	if cfg.Help || len(argParseErrors) > 0 {
		cfg.usageAndExit(argParseErrors)
	}

	return
}

func (cfg *config) usageAndExit(errorStrings []string) {

	if len(errorStrings) > 0 {
		fmt.Fprint(os.Stderr, "\nFatal error parsing arguments:\n\n")
	}

	cfg.optSet.PrintUsage(os.Stderr)

	if len(errorStrings) > 0 {
		sort.Strings(errorStrings)
		fmt.Fprintf(
			os.Stderr,
			"\nFatal error parsing arguments:\n\t%s\n\n",
			strings.Join(errorStrings, "\n\t"),
		)
		os.Exit(2)
	}

	os.Exit(0)
}
