package dagger

import (
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/ipfs/go-qringbuf"
	"github.com/ribasushi/DAGger/internal/constants"
	dgrblock "github.com/ribasushi/DAGger/internal/dagger/block"

	"github.com/ribasushi/DAGger/chunker"
	dgrchunker "github.com/ribasushi/DAGger/internal/dagger/chunker"
	"github.com/ribasushi/DAGger/internal/dagger/chunker/buzhash"
	"github.com/ribasushi/DAGger/internal/dagger/chunker/fixedsize"
	"github.com/ribasushi/DAGger/internal/dagger/chunker/padfinder"
	"github.com/ribasushi/DAGger/internal/dagger/chunker/pigz"
	"github.com/ribasushi/DAGger/internal/dagger/chunker/rabin"

	dgrcollector "github.com/ribasushi/DAGger/internal/dagger/collector"
	"github.com/ribasushi/DAGger/internal/dagger/collector/fixedcidrefsize"
	"github.com/ribasushi/DAGger/internal/dagger/collector/fixedoutdegree"
	"github.com/ribasushi/DAGger/internal/dagger/collector/noop"
	"github.com/ribasushi/DAGger/internal/dagger/collector/shrubber"
	"github.com/ribasushi/DAGger/internal/dagger/collector/trickle"

	dgrencoder "github.com/ribasushi/DAGger/internal/dagger/encoder"
	"github.com/ribasushi/DAGger/internal/dagger/encoder/unixfsv1"
)

var availableChunkers = map[string]dgrchunker.Initializer{
	"pad-finder": padfinder.NewChunker,
	"fixed-size": fixedsize.NewChunker,
	"buzhash":    buzhash.NewChunker,
	"rabin":      rabin.NewChunker,
	"pigz":       pigz.NewChunker,
}
var availableCollectors = map[string]dgrcollector.Initializer{
	"none":                noop.NewCollector,
	"shrubber":            shrubber.NewCollector,
	"fixed-cid-refs-size": fixedcidrefsize.NewCollector,
	"fixed-outdegree":     fixedoutdegree.NewCollector,
	"trickle":             trickle.NewCollector,
}
var availableNodeEncoders = map[string]dgrencoder.Initializer{
	"unixfsv1": unixfsv1.NewEncoder,
}

type dgrChunkerUnit struct {
	_         constants.Incomparabe
	instance  chunker.Chunker
	constants dgrchunker.InstanceConstants
}

type carUnit struct {
	_      constants.Incomparabe
	hdr    *dgrblock.Header
	region *qringbuf.Region
}

type Dagger struct {
	// speederization shortcut flags for internal logic
	generateRoots bool
	emitChunks    bool

	latestLeafInlined bool
	curStreamOffset   int64
	cfg               config
	statSummary       statSummary
	chainedChunkers   []dgrChunkerUnit
	chainedCollectors []dgrcollector.Collector
	formattedCid      func(*dgrblock.Header) string
	externalEventBus  chan<- IngestionEvent
	qrb               *qringbuf.QuantizedRingBuffer
	asyncWG           sync.WaitGroup
	asyncHashingBus   dgrblock.AsyncHashingBus
	mu                sync.Mutex
	seenBlocks        seenBlocks
	seenRoots         seenRoots
	carDataQueue      chan carUnit
	carWriteError     chan error
	carDataWriter     io.Writer
	carFifoDirectory  string
	carFifoData       *os.File
	carFifoPins       *os.File
}

var CheckGoroutineShutdown bool

func (dgr *Dagger) Destroy() {
	dgr.mu.Lock()
	if dgr.asyncHashingBus != nil {
		wantCount := runtime.NumGoroutine() - dgr.cfg.AsyncHashers
		close(dgr.asyncHashingBus)
		dgr.asyncHashingBus = nil

		if constants.PerformSanityChecks {
			dgr.mu.Unlock()
			// we will be checking for leaked goroutines - wait a bit for hashers to shut down
			for {
				time.Sleep(2 * time.Millisecond)
				if runtime.NumGoroutine() <= wantCount {
					break
				}
			}
			dgr.mu.Lock()
		}
	}
	dgr.qrb = nil
	dgr.mu.Unlock()
}
