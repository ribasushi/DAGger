package trickle

import (
	"fmt"
	"math"

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
			"Produces a \"side-balanced\" DAG optimized for streaming. Data blocks further\n"+
				"away from the stream start are arranged in nodes at increasing depth away\n"+
				"from the root. The rough \"placement group\" for a particular node LeafIndex\n"+
				"away from the stream start can be derived numerically via:\n"+
				"int( log( LeafIndex / MaxDirectLeaves ) / log( 1 + MaxSiblingSubgroups ) )\n"+
				"See the example program in trickle/init.go for more info.",
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

	// allocate space for ~8mil nodes (usually the result is 6 or 7)
	co.descentPrealloc = int(math.Ceil(
		math.Log((1<<23)/float64(co.MaxDirectLeaves)) / math.Log(1+float64(co.MaxSiblingSubgroups)),
	))

	return co, initErrs
}

// Complete CLI program demonstrating node placement
//
/*

package main

import (
	"fmt"
	"math"
)

const (
	totalLeaves         = 200
	maxDirectLeaves     = 4 // ipfs default is 174
	maxSiblingSubgroups = 2 // ipfs default is 4
)

func main() {
	for existingLeafCount := 0; existingLeafCount <= totalLeaves; existingLeafCount++ {

		// displaying direct leaf population is not interesting
		if (existingLeafCount % maxDirectLeaves) != 0 {
			continue
		}

		// all calculations below rely on the pre-existing leaf count ( 0-based index if you will )
		remainingLeaves := existingLeafCount
		fmt.Printf("%5s", fmt.Sprintf("#%d", remainingLeaves))

		for remainingLeaves >= maxDirectLeaves {

			descentLevel := int(
				(math.Log(float64((remainingLeaves) / maxDirectLeaves))) /
					math.Log(float64((1 + maxSiblingSubgroups))),
			)

			descentLevelMembers := maxDirectLeaves * int(math.Pow(
				float64(maxSiblingSubgroups+1),
				float64(descentLevel),
			))

			if remainingLeaves >= descentLevelMembers {
				subGroupNodeIndexAtLevel := (remainingLeaves / descentLevelMembers) + (descentLevel * maxSiblingSubgroups)
				fmt.Printf("\t%d", subGroupNodeIndexAtLevel)
			}

			remainingLeaves %= descentLevelMembers
		}
		fmt.Print("\n")
	}
}

*/
