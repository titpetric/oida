package oida

import (
	"crypto/rand"
	"time"
)

// crockford is the base32 alphabet used by ULID.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// idLength is the encoded length of a ULID.
const idLength = 26

// newID returns a lexicographically sortable ULID for the given time.
func newID(now time.Time) (string, error) {
	var data [16]byte
	ms := uint64(now.UnixMilli())
	data[0], data[1], data[2] = byte(ms>>40), byte(ms>>32), byte(ms>>24)
	data[3], data[4], data[5] = byte(ms>>16), byte(ms>>8), byte(ms)
	if _, err := rand.Read(data[6:]); err != nil {
		return "", err
	}

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
	return string(out[:]), nil
}

// validID reports whether id looks like a ULID produced by newID. It keeps
// hostile input out of lookups and out of rendered links.
func validID(id string) bool {
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
