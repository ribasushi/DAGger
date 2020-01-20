package dgrchunker

import (
	"github.com/ribasushi/DAGger/chunker"
	"github.com/ribasushi/DAGger/internal/constants"
)

type InstanceConstants struct {
	_            constants.Incomparabe
	MinChunkSize int
	MaxChunkSize int
}

type DaggerConfig struct {
	IsLastInChain bool
}

type Initializer func(
	chunkerCLISubArgs []string,
	cfg *DaggerConfig,
) (
	instance chunker.Chunker,
	constants InstanceConstants,
	initErrorStrings []string,
)
