package model

import (
	"crypto/rand"
	"strings"
	"time"
)

// crockford is the base32 alphabet used by ULID.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// idLength is the encoded length of a ULID.
const idLength = 26

// NewID returns a lexicographically sortable ULID for the given time.
func NewID(now time.Time) (string, error) {
	var data [16]byte
	ms := uint64(now.UnixMilli())
	data[0], data[1], data[2] = byte(ms>>40), byte(ms>>32), byte(ms>>24)
	data[3], data[4], data[5] = byte(ms>>16), byte(ms>>8), byte(ms)
	if _, err := rand.Read(data[6:]); err != nil {
		return "", err
	}
	return idString(data), nil
}

// TraceIDHex returns the 32 hex digit W3C trace identifier of a ULID, empty
// for a string that is no ULID. Both encode the same 128 bits.
func TraceIDHex(id string) string {
	data, ok := idBytes(id)
	if !ok {
		return ""
	}
	out := make([]byte, 0, traceIDLength)
	for _, b := range data {
		out = append(out, hexDigits[b>>4], hexDigits[b&0x0f])
	}
	return string(out)
}

// IDFromTraceID returns the ULID of a 32 hex digit W3C trace identifier,
// empty for a string that is none.
func IDFromTraceID(value string) string {
	if !ValidTraceID(value) {
		return ""
	}
	var data [16]byte
	for i := range data {
		high, low := hexValue(value[i*2]), hexValue(value[i*2+1])
		if high < 0 || low < 0 {
			return ""
		}
		data[i] = byte(high<<4 | low)
	}
	return idString(data)
}

// idString returns the ULID encoding of sixteen bytes.
func idString(data [16]byte) string {
	var out [idLength]byte
	for i := range out {
		var value byte
		for bit := range 5 {
			streamBit := i*5 + bit - 2
			value <<= 1
			if streamBit >= 0 && streamBit < 128 {
				value |= (data[streamBit/8] >> (7 - streamBit%8)) & 1
			}
		}
		out[i] = crockford[value]
	}
	return string(out[:])
}

// idBytes returns the sixteen bytes a ULID encodes, and whether it is one.
func idBytes(id string) ([16]byte, bool) {
	var data [16]byte
	if !ValidID(id) {
		return data, false
	}
	for i := range idLength {
		value := strings.IndexByte(crockford, id[i])
		for bit := range 5 {
			streamBit := i*5 + bit - 2
			if streamBit < 0 || streamBit >= 128 || value&(1<<(4-bit)) == 0 {
				continue
			}
			data[streamBit/8] |= 1 << (7 - streamBit%8)
		}
	}
	return data, true
}

// hexValue returns the value of one hex digit, or -1.
func hexValue(c byte) int {
	if index := strings.IndexByte(hexDigits, c); index >= 0 {
		return index
	}
	return -1
}

// ValidID reports whether id looks like a ULID produced by NewID. It keeps
// hostile input out of lookups and out of rendered links.
func ValidID(id string) bool {
	if len(id) != idLength {
		return false
	}
	for i := range len(id) {
		valid := false
		for j := range len(crockford) {
			if id[i] == crockford[j] {
				valid = true
				break
			}
		}
		if !valid {
			return false
		}
	}
	return true
}
