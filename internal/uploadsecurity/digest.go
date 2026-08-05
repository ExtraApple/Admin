package uploadsecurity

import (
	"crypto/sha256"
	"encoding/hex"
)

// SHA256Hex returns the lowercase hexadecimal SHA-256 digest of content.
func SHA256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
