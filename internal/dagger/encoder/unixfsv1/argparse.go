package unixfsv1

import (
	"fmt"

	dgrencoder "github.com/ribasushi/DAGger/internal/dagger/encoder"

	"github.com/ribasushi/DAGger/internal/dagger/util/argparser"
	"github.com/pborman/getopt/v2"
	"github.com/pborman/options"
)

func NewEncoder(args []string, dgrCfg *dgrencoder.DaggerConfig) (_ dgrencoder.NodeEncoder, initErrs []string) {

	e := &encoder{
		DaggerConfig: dgrCfg,
	}

	optSet := getopt.New()
	if err := options.RegisterSet("", &e.config, optSet); err != nil {
		initErrs = []string{fmt.Sprintf("option set registration failed: %s", err)}
		return
	}

	// on nil-args the "error" is the help text to be incorporated into
	// the larger help display
	if args == nil {
		initErrs = argparser.SubHelp(
			"Implements UnixFSv1, the only encoding currently rendered by IPFS gateways.\n"+
				"By default generates go-ipfs-standard, inefficient, 'Tsize'-full linknodes.",
			optSet,
		)
		return
	}

	// bail early if getopt fails
	if initErrs = argparser.Parse(args, optSet); len(initErrs) > 0 {
		return
	}

	if !optSet.IsSet("unixfs-leaf-decorator-type") {
		e.UnixFsType = -1
	} else if e.UnixFsType != 0 && e.UnixFsType != 2 {
		initErrs = append(initErrs, "when provided value of 'unixfs-leaf-decorator-type' can be only 0 or 2")
	}

	if e.LegacyCIDv0Links &&
		(e.HasherName != "sha2-256" ||
			e.HasherBits != 256) {
		initErrs = append(
			initErrs,
			"legacy CIDv0 linking requires --hash=sha2-256 and --hash-bits=256",
		)
	}

	return e, initErrs
}
