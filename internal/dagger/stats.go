package dagger

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/ipfs/go-qringbuf"
	dgrblock "github.com/ribasushi/DAGger/internal/dagger/block"
	dgrencoder "github.com/ribasushi/DAGger/internal/dagger/encoder"

	"github.com/ribasushi/DAGger/internal/constants"
	"github.com/ribasushi/DAGger/internal/util/text"
)

// the code as-written expects the steps to be numerically ordered
var textstatsDistributionPercentiles = [...]int{3, 10, 25, 50, 95}

// The bit reduction is to make the internal seen map smaller memory-wise
// That many bits are taken off the *end* of any non-identity CID
// We could remove the shortening, but for now there's no reason to, and
// as an extra benefit it makes the murmur3 case *way* easier to code
const seenHashSize = 128 / 8

type blockPostProcessResult struct {
	_ constants.Incomparabe
}

type seenRoot struct {
	order int
	cid   []byte
}

type seenBlocks map[[seenHashSize]byte]uniqueBlockStats
type seenRoots map[[seenHashSize]byte]seenRoot

func seenKey(b *dgrblock.Header) (id *[seenHashSize]byte) {
	if b == nil ||
		b.IsCidInlined() ||
		b.DummyHashed() {
		return
	}

	cid := b.Cid()
	id = new([seenHashSize]byte)
	copy(
		id[:],
		cid[(len(cid)-seenHashSize):],
	)
	return
}

type statSummary struct {
	EventType string `json:"event"`
	Dag       struct {
		Nodes   int64 `json:"nodes"`
		Size    int64 `json:"wireSize"`
		Payload int64 `json:"payload"`
	} `json:"logicalDag"`
	Streams  int64        `json:"subStreams"`
	Roots    []rootStats  `json:"roots,omitempty"`
	Layers   []layerStats `json:"layers,omitempty"`
	SysStats struct {
		qringbuf.Stats
		ElapsedNsecs int64 `json:"elapsedNanoseconds"`

		// getrusage() section
		CpuUserNsecs int64 `json:"cpuUserNanoseconds"`
		CpuSysNsecs  int64 `json:"cpuSystemNanoseconds"`
		MaxRssBytes  int64 `json:"maxMemoryUsed"`
		MinFlt       int64 `json:"cacheMinorFaults"`
		MajFlt       int64 `json:"cacheMajorFaults"`
		BioRead      int64 `json:"blockIoReads,omitempty"`
		BioWrite     int64 `json:"blockIoWrites,omitempty"`
		Sigs         int64 `json:"signalsReceived,omitempty"`
		CtxSwYield   int64 `json:"contextSwitchYields"`
		CtxSwForced  int64 `json:"contextSwitchForced"`

		// for context
		PageSize int `json:"pageSize"`
		CPU      struct {
			NameStr        string `json:"name"`
			FeaturesStr    string `json:"features"`
			Cores          int    `json:"cores"`
			ThreadsPerCore int    `json:"threadsPerCore"`
			FreqMHz        int    `json:"mhz"`
			Vendor         string `json:"vendor"`
			Family         int    `json:"family"`
			Model          int    `json:"model"`
		} `json:"cpu"`
		GoMaxProcs int    `json:"goMaxProcs"`
		Os         string `json:"os"`

		ArgvExpanded []string `json:"argvExpanded"`
		ArgvInitial  []string `json:"argvInitial"`
		GoVersion    string   `json:"goVersion"`
	} `json:"sys"`
}
type layerStats struct {
	label     string
	LongLabel string `json:"label"`

	// the map is used to construct the array for display at the very end
	countTracker    map[int]*sameSizeBlockStats
	BlockSizeCounts []sameSizeBlockStats `json:"distinctlySizedBlockCounts"`
}
type rootStats struct {
	Cid         string `json:"cid"`
	SizeDag     uint64 `json:"wireSize"`
	SizePayload uint64 `json:"payload"`
	Dup         bool   `json:"duplicate,omitempty"`
}
type sameSizeBlockStats struct {
	CountUniqueBlocksAtSize int64 `json:"count"`
	SizeBlock               int   `json:"blockSize"`
	CountRootBlocksAtSize   int64 `json:"roots,omitempty"`
}
type uniqueBlockStats struct {
	sizeBlock int
	seenAt    seenTimesAt
	*blockPostProcessResult
}
type seenTimesAt map[dgrencoder.NodeOrigin]int64

func (dgr *Dagger) OutputSummary() {

	// no stats emitters - nowhere to output
	if dgr.cfg.emitters[emStatsText] == nil && dgr.cfg.emitters[emStatsJsonl] == nil {
		return
	}

	smr := &dgr.statSummary
	var totalUCount, totalUWeight, leafUWeight, leafUCount int64

	if dgr.seenBlocks != nil && len(dgr.seenBlocks) > 0 {
		layers := make(map[dgrencoder.NodeOrigin]*layerStats, 10) // if more than 10 layers - something odd is going on

		for sk, b := range dgr.seenBlocks {
			totalUCount++
			totalUWeight += int64(b.sizeBlock)

			// An identical block could be emitted by multiple generators ( e.g. trickle could )
			// Take the lowest-sorting one
			gens := make([]dgrencoder.NodeOrigin, 0, len(b.seenAt))
			for g := range b.seenAt {
				gens = append(gens, g)
			}
			sortGenerators(gens)

			if _, exist := layers[gens[0]]; !exist {
				layers[gens[0]] = &layerStats{
					countTracker: make(map[int]*sameSizeBlockStats, 256), // SANCHECK: somewhat arbitrary
				}
			}
			if _, exist := layers[gens[0]].countTracker[b.sizeBlock]; !exist {
				layers[gens[0]].countTracker[b.sizeBlock] = &sameSizeBlockStats{
					SizeBlock: b.sizeBlock,
				}

			}
			layers[gens[0]].countTracker[b.sizeBlock].CountUniqueBlocksAtSize++

			if _, root := dgr.seenRoots[sk]; root {
				layers[gens[0]].countTracker[b.sizeBlock].CountRootBlocksAtSize++
			}
		}

		var nonLinkLayers int
		genInOrder := make([]dgrencoder.NodeOrigin, 0, len(layers))
		for g := range layers {
			genInOrder = append(genInOrder, g)
			if g.OriginatingLayer == -1 {
				nonLinkLayers++
			}
		}
		sortGenerators(genInOrder)

		for i, g := range genInOrder {
			if g.OriginatingLayer == -1 {
				if g.LocalSubLayer == 0 || g.LocalSubLayer == 1 {
					for s, c := range layers[g].countTracker {
						leafUWeight += c.CountUniqueBlocksAtSize * int64(s)
						leafUCount += c.CountUniqueBlocksAtSize
					}
				}
				if g.LocalSubLayer == 0 {
					layers[g].LongLabel = "DataBlocks"
					layers[g].label = "DB"
				} else if g.LocalSubLayer == 1 {
					layers[g].LongLabel = "PaddingBlocks"
					layers[g].label = "PB"
				} else if g.LocalSubLayer == 2 {
					layers[g].LongLabel = "PaddingSuperblocks"
					layers[g].label = "PS"
				} else {
					log.Fatalf("Unexpected leaf-local-layer '%d'", g.LocalSubLayer)
				}
			} else {
				id := len(genInOrder) - i - nonLinkLayers
				layers[g].LongLabel = fmt.Sprintf("LinkingLayer%d", id)
				layers[g].label = fmt.Sprintf("L%d", id)
			}

			for _, c := range layers[g].countTracker {
				layers[g].BlockSizeCounts = append(layers[g].BlockSizeCounts, *c)
			}
			sort.Slice(layers[g].BlockSizeCounts, func(i, j int) bool {
				return layers[g].BlockSizeCounts[i].SizeBlock < layers[g].BlockSizeCounts[j].SizeBlock
			})

			smr.Layers = append(smr.Layers, *layers[g])
		}
	}

	if statsJsonlOut := dgr.cfg.emitters[emStatsJsonl]; statsJsonlOut != nil {
		// emit the JSON last, so that piping to e.g. `jq` works nicer
		defer func() {

			// because the golang json encoder is rather garbage
			if smr.Layers == nil {
				smr.Layers = []layerStats{}
			}
			if smr.Roots == nil {
				smr.Roots = []rootStats{}
			}

			jsonl, err := json.Marshal(smr)
			if err != nil {
				log.Fatalf("Encoding '%s' failed: %s", emStatsJsonl, err)
			}

			if _, err := fmt.Fprintf(statsJsonlOut, "%s\n", jsonl); err != nil {
				log.Fatalf("Emitting '%s' failed: %s", emStatsJsonl, err)
			}
		}()
	}

	statsTextOut := dgr.cfg.emitters[emStatsText]
	if statsTextOut == nil {
		return
	}

	var substreamsDesc string
	if dgr.cfg.MultipartStream {
		substreamsDesc = fmt.Sprintf(
			" from %s substreams",
			text.Commify64(dgr.statSummary.Streams),
		)
	}

	writeTextOutf := func(f string, args ...interface{}) {
		if _, err := fmt.Fprintf(statsTextOut, f, args...); err != nil {
			log.Fatalf("Emitting '%s' failed: %s", emStatsText, err)
		}
	}

	writeTextOutf(
		"\nRan on %d-core/%d-thread %s"+
			"\nProcessing took %0.2f seconds using %0.2f vCPU and %0.2f MiB peak memory"+
			"\nPerforming %s system reads using %0.2f vCPU at about %0.2f MiB/s"+
			"\nIngesting payload of:%17s bytes%s\n\n",

		smr.SysStats.CPU.Cores,
		smr.SysStats.CPU.Cores*smr.SysStats.CPU.ThreadsPerCore,
		smr.SysStats.CPU.NameStr,

		float64(smr.SysStats.ElapsedNsecs)/
			1000000000,

		float64(smr.SysStats.CpuUserNsecs)/
			float64(smr.SysStats.ElapsedNsecs),

		float64(smr.SysStats.MaxRssBytes)/
			(1024*1024),

		text.Commify64(smr.SysStats.ReadCalls),

		float64(smr.SysStats.CpuSysNsecs)/
			float64(smr.SysStats.ElapsedNsecs),

		(float64(smr.Dag.Payload)/(1024*1024))/
			(float64(smr.SysStats.ElapsedNsecs)/1000000000),

		text.Commify64(smr.Dag.Payload),

		substreamsDesc,
	)

	if smr.Dag.Nodes > 0 {
		writeTextOutf(
			"Forming DAG covering:%17s bytes of %s logical nodes\n",
			text.Commify64(smr.Dag.Size), text.Commify64(smr.Dag.Nodes),
		)
	}

	if len(smr.Layers) == 0 {
		return
	}

	descParts := make([]string, 0, 32)

	descParts = append(descParts, fmt.Sprintf(
		"\nDataset deduped into:%17s bytes over %s unique leaf nodes\n",
		text.Commify64(leafUWeight), text.Commify64(leafUCount),
	))

	if len(smr.Layers) > 1 {
		descParts = append(descParts, fmt.Sprintf(
			"Linked as streams by:%17s bytes over %s unique DAG-PB nodes\n"+
				"Taking a grand-total:%17s bytes, ",
			text.Commify64(totalUWeight-leafUWeight), text.Commify64(totalUCount-leafUCount),
			text.Commify64(totalUWeight),
		))
	} else {
		descParts = append(descParts, fmt.Sprintf("%44s", ""))
	}

	descParts = append(descParts, fmt.Sprintf(
		"%.02f%% of original, %.01fx smaller\n"+
			` Roots\Counts\Sizes:`,
		100*float64(totalUWeight)/float64(smr.Dag.Payload),
		float64(smr.Dag.Payload)/float64(totalUWeight),
	))

	for i, val := range textstatsDistributionPercentiles {
		if i == 0 {
			descParts = append(descParts, fmt.Sprintf(" %5d%%", val))
		} else {
			descParts = append(descParts, fmt.Sprintf(" %8d%%", val))
		}
	}
	descParts = append(descParts, " |      Avg\n")

	for _, ls := range smr.Layers {
		descParts = append(descParts, distributionForLayer(ls))
	}

	writeTextOutf("%s\n", strings.Join(descParts, ""))
}

func sortGenerators(g []dgrencoder.NodeOrigin) {
	if len(g) > 1 {
		sort.Slice(g, func(i, j int) bool {
			if g[i].OriginatingLayer != g[j].OriginatingLayer {
				return g[i].OriginatingLayer > g[j].OriginatingLayer
			}
			return g[i].LocalSubLayer > g[j].LocalSubLayer
		})
	}
}

func distributionForLayer(l layerStats) (distLine string) {
	var uWeight, uCount, roots int64

	for s, c := range l.countTracker {
		uCount += c.CountUniqueBlocksAtSize
		uWeight += c.CountUniqueBlocksAtSize * int64(s)
		roots += c.CountRootBlocksAtSize
	}

	distChunks := make([][]byte, len(textstatsDistributionPercentiles))

	for i, step := range textstatsDistributionPercentiles {
		threshold := 1 + int64(float64(uCount*int64(step))/100)

		// outright skip this position if the next threshold is identical
		if i+1 < len(textstatsDistributionPercentiles) &&
			threshold == 1+int64(float64(uCount*int64(textstatsDistributionPercentiles[i+1]))/100) {
			continue
		}

		var runningCount int64
		for _, sc := range l.BlockSizeCounts {
			runningCount += sc.CountUniqueBlocksAtSize
			if runningCount >= threshold {
				distChunks[i] = text.Commify(sc.SizeBlock)
				break
			}
		}
	}

	dist := make([]byte, 0, len(distChunks)*10)
	for _, formattedSize := range distChunks {
		dist = append(dist, fmt.Sprintf(" %9s", formattedSize)...)
	}

	var layerCounts string
	if roots > 0 {
		rootStr := fmt.Sprintf("{%d}", roots)

		layerCounts = fmt.Sprintf(
			fmt.Sprintf("%%s%%%ds", 13-len(rootStr)),
			rootStr,
			text.Commify64(uCount),
		)
	} else {
		layerCounts = fmt.Sprintf("%13s", text.Commify64(uCount))
	}

	return fmt.Sprintf(
		"%s%3s:%s |%9s\n",
		layerCounts,
		l.label,
		dist,
		text.Commify64(
			uWeight/uCount,
		),
	)
}
