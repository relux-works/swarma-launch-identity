// Package identity derives public selectors from explicit stable IDs.
// Selectors authenticate nobody and grant no credential access.
package identity

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"unicode"
	"unicode/utf8"
)

const MaxIDBytes = 256
const domain = "swarma.synthetic-identity/v1\x00"

// ErrID denotes an invalid ID representation without exposing its value.
var ErrID = errors.New("invalid ID representation")

// IDs contains opaque, byte-exact UTF-8 identifiers retained by the caller.
type IDs struct {
	Host    string
	Project string
	Profile string
}

// Validate checks representation only, never provenance or uniqueness.
func (ids IDs) Validate() error {
	for _, value := range []string{ids.Host, ids.Project, ids.Profile} {
		if len(value) == 0 || len(value) > MaxIDBytes || !utf8.ValidString(value) {
			return ErrID
		}
		for _, r := range value {
			if unicode.IsControl(r) {
				return ErrID
			}
		}
	}
	return nil
}

// Encode returns the versioned domain and three uint32 big-endian length
// prefixes with the exact ID bytes. It performs no normalization or I/O.
func Encode(ids IDs) ([]byte, error) {
	if err := ids.Validate(); err != nil {
		return nil, err
	}
	encoded := make([]byte, 0, len(domain)+12+len(ids.Host)+len(ids.Project)+len(ids.Profile))
	encoded = append(encoded, domain...)
	for _, value := range []string{ids.Host, ids.Project, ids.Profile} {
		encoded = binary.BigEndian.AppendUint32(encoded, uint32(len(value)))
		encoded = append(encoded, value...)
	}
	return encoded, nil
}

// Derive returns syn1: followed by 64 lowercase SHA-256 hexadecimal digits.
func Derive(ids IDs) (string, error) {
	encoded, err := Encode(ids)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "syn1:" + hex.EncodeToString(digest[:]), nil
}
