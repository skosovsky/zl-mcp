package mobilebackup

import (
	"encoding/binary"
	"math/bits"
)

// checksum32 implements the published XXH32 algorithm with seed zero.
// It is a format checksum, not authentication.
func checksum32(input []byte) uint32 {
	const p1 uint32 = 2654435761
	const p2 uint32 = 2246822519
	const p3 uint32 = 3266489917
	const p4 uint32 = 668265263
	const p5 uint32 = 374761393
	length := len(input)
	var h uint32
	if length >= 16 {
		v1, v2, v3, v4 := uint32(606290984), p2, uint32(0), uint32(1640531535)
		round := func(v uint32, b []byte) uint32 { return bits.RotateLeft32(v+binary.LittleEndian.Uint32(b)*p2, 13) * p1 }
		for len(input) >= 16 {
			v1 = round(v1, input[:4])
			v2 = round(v2, input[4:8])
			v3 = round(v3, input[8:12])
			v4 = round(v4, input[12:16])
			input = input[16:]
		}
		h = bits.RotateLeft32(v1, 1) + bits.RotateLeft32(v2, 7) + bits.RotateLeft32(v3, 12) + bits.RotateLeft32(v4, 18)
	} else {
		h = p5
	}
	h += uint32(length)
	for len(input) >= 4 {
		h = bits.RotateLeft32(h+binary.LittleEndian.Uint32(input[:4])*p3, 17) * p4
		input = input[4:]
	}
	for _, b := range input {
		h = bits.RotateLeft32(h+uint32(b)*p5, 11) * p1
	}
	h ^= h >> 15
	h *= p2
	h ^= h >> 13
	h *= p3
	h ^= h >> 16
	return h
}
