package files

import (
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func DetectMIME(path string) string {
	if ext := strings.ToLower(filepath.Ext(path)); ext != "" {
		if t := mime.TypeByExtension(ext); t != "" {
			return t
		}
	}
	f, err := openNoFollow(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "application/octet-stream"
	}
	if n == 0 {
		return "application/octet-stream"
	}
	return http.DetectContentType(buf[:n])
}

func IsDotfile(name string) bool {
	return strings.HasPrefix(name, ".")
}

func FileUnchanged(path string, rec Record) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false
	}
	if info.Size() != rec.Size {
		return false
	}
	mod := info.ModTime().UTC().Truncate(time.Second)
	return rec.Available && rec.Hash.Digest != "" && mod.Equal(rec.ModifiedAt.UTC().Truncate(time.Second))
}
