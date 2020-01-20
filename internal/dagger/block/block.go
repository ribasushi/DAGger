package dgrblock

import (
	"fmt"
	"hash"
	"log"
	"math"
	"sync/atomic"

	sha256gocore "crypto/sha256"

	sha256simd "github.com/minio/sha256-simd"
	"github.com/twmb/murmur3"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/sha3"

	"github.com/ribasushi/DAGger/chunker"
	"github.com/ribasushi/DAGger/internal/constants"
	"github.com/ribasushi/DAGger/internal/dagger/util/encoding"
	"github.com/ribasushi/DAGger/internal/util/text"
	"github.com/ribasushi/DAGger/internal/zcpstring"
)

// multihash ids come from https://github.com/multiformats/multicodec/blob/master/table.csv
var AvailableHashers = map[string]hasher{
	"none": {
		hasherMaker: nil,
		noExport:    true,
	},
	"sha2-256": {
		multihashID: 0x12,
		hasherMaker: sha256simd.New,
	},
	"sha2-256-gocore": {
		multihashID: 0x12,
		hasherMaker: sha256gocore.New,
	},
	"sha3-512": {
		multihashID: 0x14,
		hasherMaker: sha3.New512,
	},
	"blake2b-256": {
		multihashID: 0xb220,
		hasherMaker: func() hash.Hash { hm, _ := blake2b.New256(nil); return hm },
	},
	"murmur3-128": {
		multihashID: 0x22,
		hasherMaker: func() hash.Hash { return murmur3.New128() },
		noExport:    true,
	},
}

type hasher struct {
	hasherMaker func() hash.Hash
	multihashID uint
	noExport    bool // do not allow use in car emitters
}

const (
	CodecRaw uint = 0x55
	CodecPB  uint = 0x70

	NulRootCarHeader = "\x19" + // 25 bytes of CBOR (encoded as varint :cryingbear: )
		// map with 2 keys
		"\xA2" +
		// text-key with length 5
		"\x65" + "roots" +
		// 1 element array
		"\x81" +
		// tag 42
		"\xD8\x2A" +
		// bytes with length 5
		"\x45" +
		// nul-identity-cid prefixed with \x00 as required in DAG-CBOR: https://github.com/ipld/specs/blob/master/block-layer/codecs/dag-cbor.md#links
		"\x00\x01\x55\x00\x00" +
		// text-key with length 7
		"\x67" + "version" +
		// 1, we call this v0 due to the nul-identity CID being an open question: https://github.com/ipld/go-car/issues/26#issuecomment-604299576
		"\x01"
)

type Header struct {
	// Everything in this struct needs to be "cacheable"
	// That is no data that changes without a panic can be present
	// ( e.g. stuff like "how many times was block seen in dag" is out )
	dummyHashed  bool
	isCidInlined bool
	sizeBlock    int
	// sizeCidRefs      int
	totalSizePayload uint64
	totalSizeDag     uint64
	cid              []byte
	cidReady         chan struct{}
	contentGone      *int32
	content          *zcpstring.ZcpString
}

func (h *Header) Content() (c *zcpstring.ZcpString) {
	// read first, check second
	c = h.content
	if constants.PerformSanityChecks &&
		atomic.LoadInt32(h.contentGone) != 0 {
		log.Panic("block content no longer available")
	}
	return
}
func (h *Header) EvictContent() {
	if constants.PerformSanityChecks {
		atomic.AddInt32(h.contentGone, 1)
	}
	h.content = nil
}

func (h *Header) Cid() []byte {
	<-h.cidReady

	if constants.PerformSanityChecks && !h.dummyHashed &&
		(h.cid[0] != byte(1) ||
			(len(h.cid) < 4) ||
			(h.cid[2] != byte(0) && len(h.cid) < 4+(128/8))) {
		log.Panicf(
			"block header with a seemingly invalid CID '%x' encountered",
			h.cid,
		)
	}

	return h.cid
}
func (h *Header) SizeBlock() int                { return h.sizeBlock }
func (h *Header) IsCidInlined() bool            { return h.isCidInlined }
func (h *Header) DummyHashed() bool             { return h.dummyHashed }
func (h *Header) SizeCumulativeDag() uint64     { return h.totalSizeDag }
func (h *Header) SizeCumulativePayload() uint64 { return h.totalSizePayload }

type Maker func(
	blockContent *zcpstring.ZcpString,
	codecID uint,
	sizePayload uint64,
	sizeSubDag uint64,
) *Header

type DataSource struct {
	_             constants.Incomparabe
	chunker.Chunk // critically *NOT* a reference, so that an empty DataSource{} is usable on its own
	Content       *zcpstring.ZcpString
}

type hashTask struct {
	hashBasedCidLen int
	hdr             *Header
}
type AsyncHashingBus chan<- hashTask

func MakerFromConfig(
	hashAlg string,
	cidHashSize int,
	inlineMaxSize int,
	maxAsyncHashers int,
) (maker Maker, asyncHashQueue chan hashTask, errString string) {

	hashopts, found := AvailableHashers[hashAlg]
	if !found {
		errString = fmt.Sprintf(
			"invalid hash function '%s'. Available hash names are %s",
			hashAlg,
			text.AvailableMapKeys(AvailableHashers),
		)
		return
	}

	var nativeHashSize int
	if hashopts.hasherMaker == nil {
		nativeHashSize = math.MaxInt32
	} else {
		nativeHashSize = hashopts.hasherMaker().Size()
	}

	if nativeHashSize < cidHashSize {
		errString = fmt.Sprintf(
			"selected hash function '%s' does not produce a digest satisfying the requested amount of --hash-bits '%d'",
			hashAlg,
			cidHashSize*8,
		)
		return
	}

	if maxAsyncHashers < 0 {
		errString = fmt.Sprintf(
			"invalid negative value '%d' for maxAsyncHashers",
			maxAsyncHashers,
		)
		return
	}

	// if we need to support codec ids over 127 - this will have to be switched to a map
	var codecs [128]codecMeta

	// Makes code easier to follow - in most conditionals below the CID
	// is "ready" instantly/synchronously. It is only at the very last
	// case that we spawn an actual goroutine: then we make a *new* channel
	cidPreMadeChan := make(chan struct{})
	close(cidPreMadeChan)

	var hasherSingleton hash.Hash
	if hashopts.hasherMaker != nil {

		if maxAsyncHashers == 0 {
			hasherSingleton = hashopts.hasherMaker()

		} else {
			asyncHashQueue = make(chan hashTask, 8*maxAsyncHashers) // SANCHECK queue up to 8 times the available workers

			for i := 0; i < maxAsyncHashers; i++ {
				go func() {
					hasher := hashopts.hasherMaker()
					for {
						task, chanOpen := <-asyncHashQueue
						if !chanOpen {
							return
						}
						hasher.Reset()
						task.hdr.Content().WriteTo(hasher)
						task.hdr.cid = (hasher.Sum(task.hdr.cid))[0:task.hashBasedCidLen:task.hashBasedCidLen]
						close(task.hdr.cidReady)
					}
				}()
			}
		}
	}

	maker = func(
		blockContent *zcpstring.ZcpString,
		codecID uint,
		sizeSubPayload uint64,
		sizeSubDag uint64,
	) *Header {

		if blockContent == nil {
			blockContent = &zcpstring.ZcpString{}
		}

		if constants.PerformSanityChecks && blockContent.Size() > constants.MaxBlockWireSize {
			log.Panicf(
				"size of supplied block %s exceeds the hard maximum block size %s",
				text.Commify(blockContent.Size()),
				text.Commify(constants.MaxBlockWireSize),
			)
		}

		if constants.PerformSanityChecks && codecID > 127 {
			log.Panicf(
				"codec IDs larger than 127 are not supported, however %d was supplied",
				codecID,
			)
		} else if codecs[codecID].hashedCidLength == 0 {
			initCodecMeta(&codecs[codecID], codecID, hashopts.multihashID, cidHashSize)
		}

		hdr := &Header{
			content:          blockContent,
			contentGone:      new(int32),
			cidReady:         cidPreMadeChan,
			sizeBlock:        blockContent.Size(),
			totalSizeDag:     sizeSubDag + uint64(blockContent.Size()),
			totalSizePayload: sizeSubPayload, // at present there is no payload in link-nodes
		}

		if inlineMaxSize > 0 &&
			inlineMaxSize >= hdr.sizeBlock {

			hdr.isCidInlined = true

			hdr.cid = append(
				make(
					[]byte,
					0,
					(len(codecs[codecID].identityCidPrefix)+
						encoding.VarintWireSize(uint64(hdr.sizeBlock))+
						blockContent.Size()),
				),
				codecs[codecID].identityCidPrefix...,
			)
			hdr.cid = encoding.AppendVarint(hdr.cid, uint64(hdr.sizeBlock))
			hdr.cid = blockContent.AppendTo(hdr.cid)

		} else if hashopts.hasherMaker == nil {
			hdr.dummyHashed = true
			hdr.cid = codecs[codecID].dummyCid

		} else {
			hdr.cid = append(
				make(
					[]byte,
					0,
					(len(codecs[codecID].hashedCidPrefix)+nativeHashSize),
				),
				codecs[codecID].hashedCidPrefix...,
			)

			finLen := codecs[codecID].hashedCidLength

			if asyncHashQueue == nil {
				hasherSingleton.Reset()
				blockContent.WriteTo(hasherSingleton)
				hdr.cid = (hasherSingleton.Sum(hdr.cid))[0:finLen:finLen]
			} else {
				hdr.cidReady = make(chan struct{})
				asyncHashQueue <- hashTask{
					hashBasedCidLen: finLen,
					hdr:             hdr,
				}
			}
		}

		return hdr
	}

	return
}

type codecMeta struct {
	hashedCidLength   int
	hashedCidPrefix   []byte
	identityCidPrefix []byte
	dummyCid          []byte
}

// we will do this only once per runtime per codec
// inefficiency is a-ok
func initCodecMeta(slot *codecMeta, codecID, mhID uint, cidHashSize int) {

	slot.identityCidPrefix = append(slot.identityCidPrefix, byte(1))
	slot.identityCidPrefix = encoding.AppendVarint(slot.identityCidPrefix, uint64(codecID))
	slot.identityCidPrefix = append(slot.identityCidPrefix, byte(0))

	slot.hashedCidPrefix = append(slot.hashedCidPrefix, byte(1))
	slot.hashedCidPrefix = encoding.AppendVarint(slot.hashedCidPrefix, uint64(codecID))
	slot.hashedCidPrefix = encoding.AppendVarint(slot.hashedCidPrefix, uint64(mhID))
	slot.hashedCidPrefix = encoding.AppendVarint(slot.hashedCidPrefix, uint64(cidHashSize))

	slot.hashedCidLength = len(slot.hashedCidPrefix) + cidHashSize

	// This is what we assign in case the nul hasher is selected
	// Using the exact length as an otherwise "proper" CID allows for
	// correct DAG-shape estimation
	slot.dummyCid = make(
		[]byte,
		slot.hashedCidLength,
	)
	copy(slot.dummyCid, slot.identityCidPrefix[:3])
	copy(slot.dummyCid[3:], encoding.VarintSlice(uint64(cidHashSize)))
}
