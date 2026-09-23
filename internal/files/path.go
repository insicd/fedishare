package files

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrSymlink     = errors.New("symlinks are not shared")
	ErrEscape      = errors.New("path is outside the shared folder")
	ErrNotRegular  = errors.New("not a regular file")
	ErrHidden      = errors.New("hidden path is not shared")
	ErrUnavailable = errors.New("file is not available")
)

// Policy: symlinks are rejected entirely. The share root must be a real
// directory. Every served path is resolved with Lstat / O_NOFOLLOW and must
// still sit inside that root at serve time.

func CleanShareRoot(root string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("shared directory is not set")
	}
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("shared directory must be an absolute path")
	}
	root = filepath.Clean(root)
	info, err := os.Lstat(root)
	if err != nil {
		return "", fmt.Errorf("open shared directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", ErrSymlink
	}
	if !info.IsDir() {
		return "", fmt.Errorf("shared directory is not a directory")
	}
	return root, nil
}

// RelFromRoot returns a slash-separated relative path, or an error if path
// escapes the root or is a hidden/symlink entry.
func RelFromRoot(root, abs string) (string, error) {
	root = filepath.Clean(root)
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return "", nil
	}
	if rel == ".." || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/") {
		return "", ErrEscape
	}
	for _, part := range strings.Split(rel, "/") {
		if part == ".." || part == "" {
			return "", ErrEscape
		}
		if strings.HasPrefix(part, ".") {
			return "", ErrHidden
		}
	}
	return rel, nil
}

// ResolveUnderRoot maps an untrusted relative path to an absolute path that
// still belongs to root. It rejects traversal, encoding tricks after Clean,
// and any symlink in the chain (each component is Lstat'd).
func ResolveUnderRoot(root, rel string) (string, error) {
	root, err := CleanShareRoot(root)
	if err != nil {
		return "", err
	}
	rel = strings.TrimSpace(rel)
	rel = strings.ReplaceAll(rel, "\\", "/")
	if rel == "" || rel == "." {
		return "", ErrEscape
	}
	if strings.Contains(rel, "\x00") {
		return "", ErrEscape
	}
	for _, part := range strings.Split(rel, "/") {
		if part == ".." {
			return "", ErrEscape
		}
	}
	cleaned := filepath.ToSlash(filepath.Clean("/" + rel))
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "" || cleaned == "." {
		return "", ErrEscape
	}
	if _, err := RelFromRoot(root, filepath.Join(root, filepath.FromSlash(cleaned))); err != nil {
		return "", err
	}

	cur := root
	for _, part := range strings.Split(cleaned, "/") {
		next := filepath.Join(cur, part)
		info, err := os.Lstat(next)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", ErrSymlink
		}
		if err := stillInside(root, next); err != nil {
			return "", err
		}
		cur = next
	}
	return cur, nil
}

func stillInside(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return ErrEscape
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return ErrEscape
	}
	return nil
}
