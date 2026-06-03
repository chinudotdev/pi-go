package session

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// UUIDv7 state — monotonic within a process.
var (
	uuidMu     sync.Mutex
	uuidLastTs int64 = -1 << 62
	uuidSeq    uint32
)

// UUIDv7 generates a monotonic UUIDv7 string (RFC 9562).
func UUIDv7() string {
	uuidMu.Lock()
	defer uuidMu.Unlock()

	ts := time.Now().UnixMilli()

	if ts > uuidLastTs {
		var randBuf [4]byte
		rand.Read(randBuf[:])
		uuidSeq = binary.BigEndian.Uint32(randBuf[:]) & 0x0FFFFFFF
		uuidLastTs = ts
	} else {
		uuidSeq = (uuidSeq + 1) & 0x0FFFFFFF
		if uuidSeq == 0 {
			uuidLastTs++
		}
	}

	var randTail [6]byte
	rand.Read(randTail[:])

	var b [16]byte
	b[0] = byte(uuidLastTs >> 40)
	b[1] = byte(uuidLastTs >> 32)
	b[2] = byte(uuidLastTs >> 24)
	b[3] = byte(uuidLastTs >> 16)
	b[4] = byte(uuidLastTs >> 8)
	b[5] = byte(uuidLastTs)

	// version 7 + seq high nibble
	b[6] = 0x70 | byte(uuidSeq>>24)
	b[7] = byte(uuidSeq >> 16)
	// variant 0x10 + seq
	b[8] = 0x80 | byte(uuidSeq>>8&0x3F)
	b[9] = byte(uuidSeq)

	// random tail
	b[10] = randTail[0]
	b[11] = randTail[1]
	b[12] = randTail[2]
	b[13] = randTail[3]
	b[14] = randTail[4]
	b[15] = randTail[5]

	return formatUUID(b[:])
}

func formatUUID(b []byte) string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(b[0:4]),
		binary.BigEndian.Uint16(b[4:6]),
		binary.BigEndian.Uint16(b[6:8]),
		binary.BigEndian.Uint16(b[8:10]),
		b[10:16],
	)
}

// ShortID generates a short (8-char) unique ID suitable for session entry IDs.
// Uses atomic counter for thread safety without locking.
var shortIDCounter atomic.Uint64

func ShortID() string {
	ctr := shortIDCounter.Add(1)
	var b [8]byte
	rand.Read(b[:])
	// Mix counter + random for uniqueness
	ts := uint64(time.Now().UnixMilli())
	val := ts ^ (ctr << 32) ^ binary.BigEndian.Uint64(b[:])
	return fmt.Sprintf("%08x", val&0xFFFFFFFF)
}
