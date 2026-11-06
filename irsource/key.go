package irsource

import (
	"crypto/sha256"
	"encoding/hex"
)

// keyPad is what a key is scrambled with: derived when needed, so that
// neither it nor the key sits in a binary as is.
func keyPad(n int) []byte {
	var pad []byte
	for block := byte(0); len(pad) < n; block++ {
		sum := sha256.Sum256([]byte{'a', 'r', 'm', 'r', 'e', 's', 't', '/', 's', 'b', 'x', block})
		pad = append(pad, sum[:]...)
	}
	return pad[:n]
}

// ScrambleKey is key scrambled for building into a program (with the
// linker's -X), which UnscrambleKey gives back: so that the catalog's
// key, which the app carries, does not sit in the binary as is. It keeps
// it from a casual look, not from anyone who looks for it.
func ScrambleKey(key string) string {
	b := []byte(key)
	for i, p := range keyPad(len(b)) {
		b[i] ^= p
	}
	return hex.EncodeToString(b)
}

// UnscrambleKey is the key ScrambleKey scrambled, or "" for anything
// else.
func UnscrambleKey(s string) string {
	b, err := hex.DecodeString(s)
	if err != nil {
		return ""
	}
	for i, p := range keyPad(len(b)) {
		b[i] ^= p
	}
	return string(b)
}
