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
	const (
		stripeBytes                 = 16
		laneBytes                   = 4
		secondLaneOffset            = 4
		thirdLaneOffset             = 8
		fourthLaneOffset            = 12
		initialAccumulator1  uint32 = 606290984
		initialAccumulator4  uint32 = 1640531535
		roundRotation               = 13
		accumulator2Rotation        = 7
		accumulator3Rotation        = 12
		accumulator4Rotation        = 18
		wordRotation                = 17
		byteRotation                = 11
		avalancheShift1             = 15
		avalancheShift2             = 13
		avalancheShift3             = 16
	)
	length := len(input)
	var h uint32
	if length >= stripeBytes {
		v1, v2, v3, v4 := initialAccumulator1, p2, uint32(0), initialAccumulator4
		round := func(v uint32, b []byte) uint32 {
			return bits.RotateLeft32(v+binary.LittleEndian.Uint32(b)*p2, roundRotation) * p1
		}
		for len(input) >= stripeBytes {
			v1 = round(v1, input[:laneBytes])
			v2 = round(v2, input[secondLaneOffset:thirdLaneOffset])
			v3 = round(v3, input[thirdLaneOffset:fourthLaneOffset])
			v4 = round(v4, input[fourthLaneOffset:stripeBytes])
			input = input[stripeBytes:]
		}
		h = bits.RotateLeft32(
			v1,
			1,
		) + bits.RotateLeft32(
			v2,
			accumulator2Rotation,
		) + bits.RotateLeft32(
			v3,
			accumulator3Rotation,
		) + bits.RotateLeft32(
			v4,
			accumulator4Rotation,
		)
	} else {
		h = p5
	}
	//nolint:gosec // G115: xxHash32 deliberately truncates length modulo 2^32.
	h += uint32(length)
	for len(input) >= laneBytes {
		h = bits.RotateLeft32(h+binary.LittleEndian.Uint32(input[:laneBytes])*p3, wordRotation) * p4
		input = input[laneBytes:]
	}
	for _, b := range input {
		h = bits.RotateLeft32(h+uint32(b)*p5, byteRotation) * p1
	}
	h ^= h >> avalancheShift1
	h *= p2
	h ^= h >> avalancheShift2
	h *= p3
	h ^= h >> avalancheShift3
	return h
}
