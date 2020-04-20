package rtmp

import (
	"encoding/binary"
	"errors"
)

// RTMP Chunk Header
//
// The header is broken down into three parts:
//
// | Basic header|Chunk Msg Header|Extended Time Stamp|   Chunk Data |
//
// Chunk basic header: 1 to 3 bytes
//
// This field encodes the chunk stream ID and the chunk type. Chunk
// type determines the format of the encoded message header. The
// length depends entirely on the chunk stream ID, which is a
// variable-length field.
//
// Chunk message header: 0, 3, 7, or 11 bytes
//
// This field encodes information about the message being sent
// (whether in whole or in part). The length can be determined using
// the chunk type specified in the chunk header.
//
// Extended timestamp: 0 or 4 bytes
//
// This field MUST be sent when the normal timsestamp is set to
// 0xffffff, it MUST NOT be sent if the normal timestamp is set to
// anything else. So for values less than 0xffffff the normal
// timestamp field SHOULD be used in which case the extended timestamp
// MUST NOT be present. For values greater than or equal to 0xffffff
// the normal timestamp field MUST NOT be used and MUST be set to
// 0xffffff and the extended timestamp MUST be sent.

type Header struct {
	Fmt           uint8
	ChunkStreamID uint32

	Timestamp       uint32
	MessageLength   uint32
	MessageTypeID   uint8
	MessageStreamID uint32

	// Extended Timestamp
	// This field is transmitted only when the normal time stamp in the
	// chunk message header is set to 0x00ffffff. If normal time stamp is
	// set to any value less than 0x00ffffff, this field MUST NOT be
	// present. This field MUST NOT be present if the timestamp field is not
	// present. Type 3 chunks MUST NOT have this field.
	ExtendTimeStamp uint32
}

func ReadBaseHeader(r Reader) (n int, fmt uint8, csi uint32, err error) {
	var b byte
	if b, err = ReadByteFromNetwork(r); err != nil {
		return
	}
	n = 1
	fmt = uint8(b >> 6)
	b = b & 0x3f
	switch b {
	case 0:
		// Chunk stream IDs 64-319 can be encoded in the 2-byte version of this
		// field. ID is computed as (the second byte + 64).
		if b, err = ReadByteFromNetwork(r); err != nil {
			return
		}
		n += 1
		csi = uint32(64) + uint32(b)
	case 1:
		// Chunk stream IDs 64-65599 can be encoded in the 3-byte version of
		// this field. ID is computed as ((the third byte)*256 + the second byte
		// + 64).
		if b, err = ReadByteFromNetwork(r); err != nil {
			return
		}
		n += 1
		csi = uint32(64) + uint32(b)
		if b, err = ReadByteFromNetwork(r); err != nil {
			return
		}
		n += 1
		csi += uint32(b) * 256
	default:
		// Chunk stream IDs 2-63 can be encoded in the 1-byte version of this
		// field.
		csi = uint32(b)
	}
	return
}

func (header *Header) ReadHeader(r Reader, vfmt uint8, csi uint32, lastHeader *Header) (n int, err error) {
	header.Fmt = vfmt
	header.ChunkStreamID = csi
	var b byte
	tmpBuf := make([]byte, 4)

	if header.Fmt < HEADER_FMT_CONTINUATION {
		// timestamp delta
		if _, err = ReadAtLeastFromNetwork(r, tmpBuf[1:], 3); err != nil {
			return
		}
		n += 3
		header.Timestamp = binary.BigEndian.Uint32(tmpBuf)
	}
	if header.Fmt < HEADER_FMT_SAME_LENGTH_AND_STREAM {
		if _, err = ReadAtLeastFromNetwork(r, tmpBuf[1:], 3); err != nil {
			return
		}
		n += 3
		header.MessageLength = binary.BigEndian.Uint32(tmpBuf)
		if b, err = ReadByteFromNetwork(r); err != nil {
			return
		}
		n += 1
		header.MessageTypeID = uint8(b)
	}
	if header.Fmt < HEADER_FMT_SAME_STREAM {
		if _, err = ReadAtLeastFromNetwork(r, tmpBuf, 4); err != nil {
			return
		}
		n += 4
		header.MessageStreamID = binary.LittleEndian.Uint32(tmpBuf)
	}

	if (header.Fmt != HEADER_FMT_CONTINUATION && header.Timestamp >= 0xffffff) ||
		(header.Fmt == HEADER_FMT_CONTINUATION && lastHeader != nil && lastHeader.ExtendTimeStamp > 0) {
		if _, err = ReadAtLeastFromNetwork(r, tmpBuf, 4); err != nil {
			return
		}
		n += 4
		header.ExtendTimeStamp = binary.BigEndian.Uint32(tmpBuf)
	} else {
		header.ExtendTimeStamp = 0
	}
	return
}

func (header *Header) Write(w Writer) (n int, err error) {
	switch {
	case header.ChunkStreamID <= 63:
		if err = w.WriteByte(byte(header.Fmt<<6) | byte(header.ChunkStreamID)); err != nil {
			return
		}
		n += 1
	case header.ChunkStreamID <= 319:
		if err = w.WriteByte(header.Fmt << 6); err != nil {
			return
		}
		n += 1
		if err = w.WriteByte(byte(header.ChunkStreamID - 64)); err != nil {
			return
		}
		n += 1
	case header.ChunkStreamID <= 65599:
		if err = w.WriteByte((header.Fmt << 6) | 0x01); err != nil {
			return
		}
		n += 1
		tmp := uint16(header.ChunkStreamID - 64)
		if err = binary.Write(w, binary.BigEndian, &tmp); err != nil {
			return
		}
		n += 2
	default:
		return n, errors.New("Unsupport chunk stream ID larger than 65599")
	}
	tmpBuf := make([]byte, 4)
	var m int
	if header.Fmt < HEADER_FMT_CONTINUATION {
		// Write Timestamp
		binary.BigEndian.PutUint32(tmpBuf, header.Timestamp)
		if m, err = w.Write(tmpBuf[1:]); err != nil {
			return
		}
		n += m
	}
	if header.Fmt < HEADER_FMT_SAME_LENGTH_AND_STREAM {
		// Write Message Length
		binary.BigEndian.PutUint32(tmpBuf, header.MessageLength)
		if m, err = w.Write(tmpBuf[1:]); err != nil {
			return
		}
		n += m
		// Write Message Type
		if err = w.WriteByte(header.MessageTypeID); err != nil {
			return
		}
		n += 1
	}
	if header.Fmt < HEADER_FMT_SAME_STREAM {
		// Write Message Stream ID
		if err = binary.Write(w, binary.LittleEndian, &(header.MessageStreamID)); err != nil {
			return
		}
		n += 4
	}

	if header.Timestamp >= 0xffffff {
		if err = binary.Write(w, binary.BigEndian, &(header.ExtendTimeStamp)); err != nil {
			return
		}
		n += 4
	}
	return
}

func (header *Header) RealTimestamp() uint32 {
	if header.Timestamp >= 0xffffff {
		return header.ExtendTimeStamp
	}
	return header.Timestamp
}
