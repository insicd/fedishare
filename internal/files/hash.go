package files

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

// HashReader streams r and returns a SHA-256 content identity.
func HashReader(r io.Reader) (ContentID, error) {
	h := sha256.New()
	buf := make([]byte, 32*1024)
	if _, err := io.CopyBuffer(h, r, buf); err != nil {
		return ContentID{}, fmt.Errorf("hash contents: %w", err)
	}
	return ContentID{Algorithm: AlgoSHA256, Digest: hex.EncodeToString(h.Sum(nil))}, nil
}

// HashFile hashes a regular file using streaming I/O. The path must already
// have been validated as a non-symlink inside the share root.
func HashFile(path string) (ContentID, error) {
	f, err := openNoFollow(path)
	if err != nil {
		return ContentID{}, err
	}
	defer f.Close()
	return HashReader(f)
}
