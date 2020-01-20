package buzhash

import (
	"math/bits"

	"github.com/ribasushi/DAGger/chunker"
)

type config struct {
	TargetValue uint32 `getopt:"--state-target=uint32     State value denoting a chunk boundary (IPFS default: 0)"`
	MaskBits    int    `getopt:"--state-mask-bits=[5:22]  Amount of bits of state to compare to target on every iteration. For random input average chunk size is about 2**m (IPFS default: 17)"`
	MaxSize     int    `getopt:"--max-size=[1:MaxPayload] Maximum data chunk size (IPFS default: 524288)"`
	MinSize     int    `getopt:"--min-size=[0:MaxPayload] Minimum data chunk size (IPFS default: 131072)"`
	xvName      string // getopt attached dynamically during init
}

type buzhashChunker struct {
	// derived from the tables at the end of the file, selectable via --hash-table
	mask           uint32
	target         uint32
	minSansPreheat int
	xv             xorVector
	config
}
type xorVector [256]uint32

func (c *buzhashChunker) Split(
	buf []byte,
	useEntireBuffer bool,
	cb chunker.SplitResultCallback,
) (err error) {

	var state uint32
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
		state = 0

		// preheat
		curIdx += c.minSansPreheat
		for i := 0; i < 32; i++ {
			state = bits.RotateLeft32(state, 1) ^ c.xv[buf[curIdx]]
			curIdx++
		}

		// cycle
		for curIdx < nextRoundMax && ((state & c.mask) != c.target) {
			// it seems we are skipping one rotation compared to what asuran does
			// https://gitlab.com/asuran-rs/asuran/-/blob/06206d116259821aded5ab1ee2897655b1724c69/asuran-chunker/src/buzhash.rs#L93
			state = bits.RotateLeft32(state, 1) ^ c.xv[buf[curIdx]] ^ c.xv[buf[curIdx-32]]
			curIdx++
		}

		// always a find at this point, we bailed on short buffers earlier
		err = cb(chunker.Chunk{Size: curIdx - lastIdx})
		if err != nil {
			return
		}
	}
}
