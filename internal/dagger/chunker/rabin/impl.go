package rabin

import (
	"github.com/ribasushi/DAGger/chunker"
	"github.com/ribasushi/DAGger/internal/dagger/chunker/rabin/bootstrap"
)

type config struct {
	Polynomial  uint64 `getopt:"--polynomial=uint64      (IPFS default: 17437180132763653)"`
	TargetValue uint64 `getopt:"--state-target=uint64    State value denoting a chunk boundary (IPFS default: 0)"`
	MaskBits    int    `getopt:"--state-mask-bits=[5:22] Amount of bits of state to compare to target on every iteration. For random input average chunk size is about 2**m (IPFS default: 18)"`
	WindowSize  int    `getopt:"--window-size=bytes    State value denoting a chunk boundary (IPFS default: 16)"`
	MaxSize     int    `getopt:"--max-size=[1:MaxPayload]        Maximum data chunk size (IPFS default: 393216)"`
	MinSize     int    `getopt:"--min-size=[0:MaxPayload]        Minimum data chunk size (IPFS default: 87381)"`
}

type rabinChunker struct {
	// settings brought from the selected preset
	initState      uint64
	mask           uint64
	minSansPreheat int
	outTable       [256]uint64
	modTable       [256]uint64
	config
}

func (c *rabinChunker) Split(
	buf []byte,
	useEntireBuffer bool,
	cb chunker.SplitResultCallback,
) (err error) {

	var state uint64
	var curIdx, lastIdx, nextRoundMax int
	postBufIdx := len(buf)

	for {
		lastIdx = curIdx
		nextRoundMax = lastIdx + c.MaxSize

		// we will be running out of data, but still *could* run a round
		if nextRoundMax > postBufIdx {
			// abort early if we are allowed to
			if !useEntireBuffer {
				return
			}
			// otherwise signify where we stop hard
			nextRoundMax = postBufIdx
		}

		// in case we will *NOT* be able to run another round at all
		if curIdx+c.MinSize >= postBufIdx {
			if useEntireBuffer && postBufIdx != curIdx {
				err = cb(chunker.Chunk{Size: postBufIdx - curIdx})
			}
			return
		}

		// reset
		state = c.initState

		// preheat
		curIdx += c.minSansPreheat
		for i := 1; i <= c.WindowSize; i++ {
			if i == c.WindowSize {
				state ^= c.outTable[1]
			} else {
				state ^= c.outTable[0]
			}
			state = (state << 8) | uint64(buf[curIdx]) ^ (c.modTable[state>>bootstrap.DegShift])

			curIdx++
		}

		// cycle
		for curIdx < nextRoundMax && ((state & c.mask) != c.TargetValue) {
			state ^= c.outTable[buf[curIdx-c.WindowSize]]
			state = (state << 8) | uint64(buf[curIdx]) ^ (c.modTable[state>>bootstrap.DegShift])
			curIdx++
		}

		// always a find at this point, we bailed on short buffers earlier
		err = cb(chunker.Chunk{Size: curIdx - lastIdx})
		if err != nil {
			return
		}
	}
}
