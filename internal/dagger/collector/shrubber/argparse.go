package shrubber

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
			"This collector allows one to arrange, group and emit nodes as smaller\n"+
				"subtrees (shrubberies), before passing them to the next collector in the\n"+
				"chain. It combines several modes of operation, each benefitting from being\n"+
				"as close to the 'leaf node' layer as possible. Specifically:\n"+
				" - ",
			optSet,
		)
		return
	}

	// bail early if getopt fails
	if initErrs = argparser.Parse(args, optSet); len(initErrs) > 0 {
		return
	}

	if co.ChainPosition != 1 {
		initErrs = append(initErrs, "collector must be first in chain")
	}
	if co.NextCollector == nil {
		initErrs = append(initErrs, "collector can not be last in chain")
	}

	co.cidMask = (1 << uint(co.SubgroupCidMaskBits)) - 1
	co.cidTailTarget = uint16(co.SubgroupCidTarget)

	return co, initErrs
}
