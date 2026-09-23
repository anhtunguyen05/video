package ids

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewUUID returns an RFC 9562 version 4 UUID without adding a third-party dependency.
func NewUUID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate uuid: %w", err)
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	encoded := make([]byte, 36)
	hex.Encode(encoded[0:8], bytes[0:4])
	hex.Encode(encoded[9:13], bytes[4:6])
	hex.Encode(encoded[14:18], bytes[6:8])
	hex.Encode(encoded[19:23], bytes[8:10])
	hex.Encode(encoded[24:36], bytes[10:16])
	encoded[8] = '-'
	encoded[13] = '-'
	encoded[18] = '-'
	encoded[23] = '-'
	return string(encoded), nil
}

func IsUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	for index, character := range []byte(value) {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F')) {
			return false
		}
	}
	return true
}
