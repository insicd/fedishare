// Package files inspects the user-selected share root without following
// symlinks. Phase 3 will add hashing and the persistent index.
package files

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const defaultListLimit = 200

// Inspect walks shareRoot. The root itself must be a real directory, not a symlink.
func Inspect(shareRoot string, listLimit int) (Summary, []Entry, error) {
	if shareRoot == "" {
		return Summary{}, nil, fmt.Errorf("shared directory is not set")
	}
	if !filepath.IsAbs(shareRoot) {
		return Summary{}, nil, fmt.Errorf("shared directory must be an absolute path")
	}
	shareRoot = filepath.Clean(shareRoot)

	info, err := os.Lstat(shareRoot)
	if err != nil {
		return Summary{}, nil, fmt.Errorf("open shared directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Summary{}, nil, fmt.Errorf("shared directory cannot be a symlink")
	}
	if !info.IsDir() {
		return Summary{}, nil, fmt.Errorf("shared directory is not a directory")
	}

	if listLimit <= 0 {
		listLimit = defaultListLimit
	}

	var sum Summary
	var list []Entry
	err = filepath.WalkDir(shareRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == shareRoot {
			return nil
		}
		rel, err := filepath.Rel(shareRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || strings.HasPrefix(rel, "../") {
			return fmt.Errorf("path escaped shared directory")
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		sum.Files++
		sum.Bytes += info.Size()
		if len(list) < listLimit {
			list = append(list, Entry{
				RelativePath: rel,
				Name:         name,
				Size:         info.Size(),
			})
		}
		return nil
	})
	if err != nil {
		return Summary{}, nil, err
	}
	return sum, list, nil
}
