package fixedcidrefsize

import (
	"fmt"

	dgrcollector "github.com/ribasushi/DAGger/internal/dagger/collector"

	"github.com/pborman/getopt/v2"
	"github.com/pborman/options"
	"github.com/ribasushi/DAGger/internal/dagger/util/argparser"
)

func NewCollector(args []string, dgrCfg *dgrcollector.DaggerConfig) (_ dgrcollector.Collector, initErrs []string) {

	co := &collector{
		DaggerConfig: dgrCfg,
		state:        state{stack: []*layer{{}}},
	}

	optSet := getopt.New()
	if err := options.RegisterSet("", &co.config, optSet); err != nil {
		initErrs = []string{fmt.Sprintf("option set registration failed: %s", err)}
		return
	}

	// on nil-args the "error" is the help text to be incorporated into
	// the larger help display
	if args == nil {
		initErrs = argparser.SubHelp(
			"Forms a DAG where the amount of bytes taken by CID references is limited\n"+
				"for every individual node. The IPLD-link referencing overhead, aside from\n"+
				"the CID length itself, is *NOT* considered.",
			optSet,
		)
		return
	}

	// bail early if getopt fails
	if initErrs = argparser.Parse(args, optSet); len(initErrs) > 0 {
		return
	}

	if co.NextCollector != nil {
		initErrs = append(
			initErrs,
			"collector must appear last in chain",
		)
	}

	return co, initErrs
}
