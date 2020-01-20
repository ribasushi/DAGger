package bootstrap

import (
	"fmt"
	"math/bits"
)

const (
	degTarget = uint(53) // Largest prime smaller than (64 - 8)
	DegShift  = uint(degTarget - 8)
)

func GenerateLookupTables(pol uint64, wSize int) (outTable, modTable [256]uint64, err error) {

	if deg(pol) != int(degTarget) {
		err = fmt.Errorf(
			"polynomial '%d' of degree %d provided, but degree %d expected",
			pol,
			deg(pol),
			degTarget,
		)
		return
	}

	if wSize < 8 {
		err = fmt.Errorf(
			"window size '%d' must be at least 8 bytes",
			wSize,
		)
		return
	}

	// calculate table for sliding out bytes. The byte to slide out is used as
	// the index for the table, the value contains the following:
	// out_table[b] = Hash(b || 0 ||        ...        || 0)
	//                          \ windowsize-1 zero bytes /
	// To slide out byte b_0 for window size w with known hash
	// H := H(b_0 || ... || b_w), it is sufficient to add out_table[b_0]:
	//    H(b_0 || ... || b_w) + H(b_0 || 0 || ... || 0)
	//  = H(b_0 + b_0 || b_1 + 0 || ... || b_w + 0)
	//  = H(    0     || b_1 || ...     || b_w)
	//
	// Afterwards a new byte can be shifted in.
	for b := uint64(0); b < 256; b++ {
		h := mod(b, pol)
		for i := 0; i < wSize-1; i++ {
			h = mod(h<<8, pol)
		}
		outTable[b] = h
	}

	// calculate table for reduction mod Polynomial
	// mod_table[b] = A | B, where A = (b(x) * x^k mod pol) and  B = b(x) * x^k
	//
	// The 8 bits above deg(Polynomial) determine what happens next and so
	// these bits are used as a lookup to this table. The value is split in
	// two parts: Part A contains the result of the modulus operation, part
	// B is used to cancel out the 8 top bits so that one XOR operation is
	// enough to reduce modulo Polynomial
	for b := uint64(0); b < 256; b++ {
		modTable[b] = mod(b<<degTarget, pol) | b<<degTarget
	}

	return
}

// the degree of the polynomial pol. If pol is zero, -1 is returned.
func deg(pol uint64) int {
	return bits.Len64(pol) - 1
}

// the modulus result of numerator%denominator
// see https://en.wikipedia.org/wiki/Division_algorithm
func mod(numerator, denominator uint64) uint64 {
	if denominator == 0 {
		panic("division by zero")
	}

	if numerator == 0 {
		return 0
	}

	denomDeg := deg(denominator)

	degDiff := deg(numerator) - denomDeg
	for degDiff >= 0 {
		numerator ^= (denominator << uint(degDiff))
		degDiff = deg(numerator) - denomDeg
	}

	return numerator
}
