package noop

import (
	dgrcollector "github.com/ribasushi/DAGger/internal/dagger/collector"

	"github.com/ribasushi/DAGger/internal/dagger/util/argparser"
)

func NewCollector(args []string, dgrCfg *dgrcollector.DaggerConfig) (_ dgrcollector.Collector, initErrs []string) {

	if args == nil {
		initErrs = argparser.SubHelp(
			"Does not form a DAG, nor emits a root CID. Simply redirects chunked data\n"+
				"to /dev/null. Takes no arguments.\n",
			nil,
		)
		return
	}

	if len(args) > 1 {
		initErrs = append(initErrs, "collector takes no arguments")
	}
	if dgrCfg.NextCollector != nil {
		initErrs = append(initErrs, "collector must appear last in chain")
	}

	return &nulCollector{dgrCfg}, initErrs
}
