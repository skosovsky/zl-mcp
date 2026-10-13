package mobilebackup

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"

	"github.com/ulikunitz/xz"
)

const (
	maxXZDictionary               = 64 << 20
	maxXZStreams                  = 100
	maxXZBlocks                   = 1000
	xzInitialDictionaryCapacity   = 4096
	xzMinimumStreamBytes          = 32
	xzAlignmentBytes              = 4
	xzAlignmentMask               = 3
	xzStreamBoundaryBytes         = 12
	xzMinimumIndexBytes           = 8
	xzMinimumBlockHeaderBytes     = 8
	xzStreamBoundariesBytes       = 24
	xzMinimumRecordBytes          = 5
	xzCRC32Bytes                  = 4
	xzCRC64Bytes                  = 8
	xzSHA256Bytes                 = 32
	xzCheckCRC64                  = 4
	xzCheckSHA256                 = 10
	lzmaMaximumDictionaryProperty = 40
	lzmaDictionaryBaseExponent    = 11
	xzVLIBytePayloadBits          = 7
	xzVLIBytePayloadMask          = (1 << xzVLIBytePayloadBits) - 1
)

var ErrXZ = errors.New("invalid mobile backup compressed stream")

// DecompressXZ validates allocation declarations before invoking the decoder.
// It does not validate archive ownership or persist any records.
func DecompressXZ(ctx context.Context, r io.Reader, compressedLimit, outputLimit int64) ([]byte, error) {
	if ctx == nil || r == nil || compressedLimit <= 0 || outputLimit <= 0 || uint64(compressedLimit) > MaxTotalBytes ||
		uint64(outputLimit) > MaxTotalBytes {
		return nil, ErrXZ
	}
	data, err := io.ReadAll(io.LimitReader(cancelReader{ctx, r}, compressedLimit+1))
	defer clear(data)
	if err != nil || int64(len(data)) > compressedLimit {
		return nil, ErrXZ
	}
	declared, err := inspectXZ(ctx, data, uint64(outputLimit))
	if err != nil {
		return nil, ErrXZ
	}
	//nolint:exhaustruct_v5 // Keep decoder defaults for omitted options; the validated dictionary cap is explicit.
	decoder, err := (xz.ReaderConfig{DictCap: xzInitialDictionaryCapacity}).NewReader(
		cancelReader{ctx, bytes.NewReader(data)},
	)
	if err != nil {
		return nil, ErrXZ
	}
	out, err := io.ReadAll(io.LimitReader(cancelReader{ctx, decoder}, outputLimit+1))
	if err != nil || ctx.Err() != nil || uint64(len(out)) != declared || int64(len(out)) > outputLimit {
		clear(out)
		return nil, ErrXZ
	}
	return out, nil
}

type xzRecord struct{ unpadded, output uint64 }

// inspectXZ walks complete stream indexes before any dictionary can be allocated.
//
//nolint:cyclop,funlen,gocognit,gocyclo // This bounded protocol parser validates complete framing/records before publishing and retains its buffer-clearing error paths.
func inspectXZ(ctx context.Context, data []byte, outputLimit uint64) (uint64, error) {
	if len(data) < xzMinimumStreamBytes || len(data)%xzAlignmentBytes != 0 {
		return 0, ErrXZ
	}
	end, streams, blocks := len(data), 0, 0
	var output uint64
	for end > 0 {
		if ctx.Err() != nil {
			return 0, ErrXZ
		}
		padding := 0
		for end > 0 && data[end-1] == 0 {
			end--
			padding++
		}
		if padding%xzAlignmentBytes != 0 || end < xzMinimumStreamBytes || streams >= maxXZStreams {
			return 0, ErrXZ
		}
		streams++
		footer := data[end-xzStreamBoundaryBytes : end]
		if string(footer[10:]) != "YZ" || crc32.ChecksumIEEE(footer[4:10]) != binary.LittleEndian.Uint32(footer[:4]) ||
			footer[8] != 0 {
			return 0, ErrXZ
		}
		checkBytes := 0
		switch footer[9] {
		case 0:
		case 1:
			checkBytes = xzCRC32Bytes
		case xzCheckCRC64:
			checkBytes = xzCRC64Bytes
		case xzCheckSHA256:
			checkBytes = xzSHA256Bytes
		default:
			return 0, ErrXZ
		}
		indexLen := (uint64(binary.LittleEndian.Uint32(footer[4:8])) + 1) * xzAlignmentBytes
		if indexLen < xzMinimumIndexBytes || indexLen > uint64(end-xzStreamBoundariesBytes) {
			return 0, ErrXZ
		}
		//nolint:gosec // G115: indexLen <= end-xzStreamBoundariesBytes, with end >= 32 derived from len(data).
		indexStart := end - xzStreamBoundaryBytes - int(indexLen)
		index := data[indexStart : end-xzStreamBoundaryBytes]
		if !xzCRC(index) || index[0] != 0 {
			return 0, ErrXZ
		}
		body := index[1 : len(index)-4]
		count, ok := xzVLI(&body)
		if !ok || count > uint64(maxXZBlocks-blocks) {
			return 0, ErrXZ
		}
		//nolint:gosec // G115: count <= maxXZBlocks-blocks <= 1000 before conversion.
		blocks += int(count)
		//nolint:gosec // G115: count <= maxXZBlocks-blocks <= 1000 before conversion.
		records := make([]xzRecord, 0, int(count))
		var blockBytes uint64
		for range count {
			u, ok := xzVLI(&body)
			//nolint:gosec // G115: validated indexLen <= end-xzStreamBoundariesBytes makes indexStart >= 12 before unsigned conversion.
			if !ok || u < xzMinimumRecordBytes || u > uint64(indexStart) {
				return 0, ErrXZ
			}
			n, ok := xzVLI(&body)
			if !ok || n > outputLimit-output {
				return 0, ErrXZ
			}
			output += n
			blockBytes += (u + xzAlignmentMask) &^ uint64(xzAlignmentMask)
			//nolint:gosec // G115: validated indexLen <= end-xzStreamBoundariesBytes makes indexStart >= 12 before unsigned conversion.
			if blockBytes > uint64(indexStart) {
				return 0, ErrXZ
			}
			records = append(records, xzRecord{u, n})
		}
		//nolint:gosec // G115: validated indexLen <= end-xzStreamBoundariesBytes makes indexStart >= 12 before unsigned conversion.
		if len(body) > xzAlignmentMask || !xzZeros(body) || blockBytes+xzStreamBoundaryBytes > uint64(indexStart) {
			return 0, ErrXZ
		}
		start := indexStart - int(blockBytes) - xzStreamBoundaryBytes
		header := data[start : start+xzStreamBoundaryBytes]
		if !bytes.Equal(header[:6], []byte{0xfd, '7', 'z', 'X', 'Z', 0}) || !xzCRC(header[6:]) ||
			!bytes.Equal(header[6:8], footer[8:10]) {
			return 0, ErrXZ
		}
		pos := start + xzStreamBoundaryBytes
		for _, record := range records {
			if ctx.Err() != nil {
				return 0, ErrXZ
			}
			padded := (record.unpadded + xzAlignmentMask) &^ uint64(xzAlignmentMask)
			//nolint:gosec // G115: short-circuit pos <= indexStart proves the remaining length is non-negative.
			if pos > indexStart || padded > uint64(indexStart-pos) {
				return 0, ErrXZ
			}
			//nolint:gosec // G115: padded size is bounded by the remaining int-sized input region above.
			size := int(padded)
			if !inspectXZBlock(data[pos:pos+size], record, checkBytes) {
				return 0, ErrXZ
			}
			pos += size
		}
		if pos != indexStart {
			return 0, ErrXZ
		}
		end = start
	}
	if streams == 0 {
		return 0, ErrXZ
	}
	return output, nil
}

//nolint:gocognit // This bounded protocol parser validates complete framing/records before publishing and retains its buffer-clearing error paths.
func inspectXZBlock(block []byte, record xzRecord, checkBytes int) bool {
	if len(block) == 0 || block[0] == 0 {
		return false
	}
	headerLen := (int(block[0]) + 1) * xzAlignmentBytes
	//nolint:gosec // G115: byte-derived headerLen <= 1024 and checkBytes is one of 0,4,8,32.
	if headerLen < xzMinimumBlockHeaderBytes || uint64(headerLen+checkBytes) >= record.unpadded ||
		headerLen > len(block) {
		return false
	}
	header := block[:headerLen]
	if !xzCRC(header) || header[1]&0x3f != 0 {
		return false
	}
	body := header[2 : headerLen-4]
	//nolint:gosec // G115: byte-derived headerLen <= 1024 and checkBytes is one of 0,4,8,32.
	compressed := record.unpadded - uint64(headerLen+checkBytes)
	if header[1]&0x40 != 0 {
		n, ok := xzVLI(&body)
		if !ok || n != compressed {
			return false
		}
	}
	if header[1]&0x80 != 0 {
		n, ok := xzVLI(&body)
		if !ok || n != record.output {
			return false
		}
	}
	id, ok := xzVLI(&body)
	if !ok || id != 0x21 {
		return false
	}
	n, ok := xzVLI(&body)
	if !ok || n != 1 || len(body) < 1 {
		return false
	}
	property := body[0]
	if property > lzmaMaximumDictionaryProperty {
		return false
	}
	var dictionary uint64
	if property == lzmaMaximumDictionaryProperty {
		dictionary = 0xffffffff
	} else {
		dictionary = uint64(2|(property&1)) << (property/2 + lzmaDictionaryBaseExponent)
	}
	return dictionary <= maxXZDictionary && xzZeros(body[1:])
}

// xzVLI accepts only canonical integers of at most 63 bits.
func xzVLI(data *[]byte) (uint64, bool) {
	var value uint64
	for i := 0; i < 9 && i < len(*data); i++ {
		b := (*data)[i]
		value |= uint64(b&xzVLIBytePayloadMask) << (xzVLIBytePayloadBits * i)
		if b&0x80 == 0 {
			if i > 0 && b == 0 {
				return 0, false
			}
			*data = (*data)[i+1:]
			return value, true
		}
	}
	return 0, false
}

func xzCRC(data []byte) bool {
	return len(data) >= 4 && crc32.ChecksumIEEE(data[:len(data)-4]) == binary.LittleEndian.Uint32(data[len(data)-4:])
}
func xzZeros(data []byte) bool {
	for _, b := range data {
		if b != 0 {
			return false
		}
	}
	return true
}
