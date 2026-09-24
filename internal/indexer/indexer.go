package indexer

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fedishare/fedishare/internal/files"
	"github.com/fedishare/fedishare/internal/status"
)

const (
	stabilityWait = 250 * time.Millisecond
	maxStableTry  = 6
)

const (
	ChangeCreate = "create"
	ChangeUpdate = "update"
	ChangeDelete = "delete"
)

// Change is a share-index mutation that federation may publish.
type Change struct {
	Kind string
	Rec  files.Record
}

type Indexer struct {
	store  *files.Store
	root   func() string
	log    *slog.Logger
	status *status.Service
	hook   func(context.Context, Change)

	mu sync.Mutex
}

func New(store *files.Store, root func() string, st *status.Service, log *slog.Logger) *Indexer {
	if log == nil {
		log = slog.Default()
	}
	return &Indexer{store: store, root: root, status: st, log: log}
}

func (idx *Indexer) Store() *files.Store { return idx.store }

func (idx *Indexer) SetHook(fn func(context.Context, Change)) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.hook = fn
}

func (idx *Indexer) emit(ctx context.Context, kind string, rec files.Record) {
	if idx.hook == nil || rec.ID == "" {
		return
	}
	idx.hook(ctx, Change{Kind: kind, Rec: rec})
}

func (idx *Indexer) Scan(ctx context.Context) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	root, err := files.CleanShareRoot(idx.root())
	if err != nil {
		return err
	}

	known, err := idx.store.KnownPaths(ctx)
	if err != nil {
		return err
	}
	seenFiles := map[string]struct{}{}
	seenDirs := map[string]struct{}{}

	var total int
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || path == root {
			return walkErr
		}
		if d.Name() != "" && d.Name()[0] == '.' {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
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
		if !d.IsDir() {
			total++
		}
		return nil
	})

	done := 0
	idx.setProgress(total, 0)

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := files.RelFromRoot(root, path)
		if err != nil {
			if d.IsDir() && errors.Is(err, files.ErrHidden) {
				return fs.SkipDir
			}
			return nil
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
		if d.IsDir() {
			seenDirs[rel] = struct{}{}
			_ = idx.store.UpsertDir(ctx, files.DirRecord{
				RelativePath: rel,
				Name:         d.Name(),
				Available:    true,
			})
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		seenFiles[rel] = struct{}{}
		if existing, ok := known[rel]; ok && files.FileUnchanged(path, existing) {
			if !existing.Available {
				existing.Available = true
				if rec, err := idx.store.Upsert(ctx, existing); err == nil {
					idx.emit(ctx, ChangeCreate, rec)
				}
			}
			done++
			idx.setProgress(total, done)
			return nil
		}
		if err := idx.indexFileLocked(ctx, root, rel, path); err != nil {
			idx.log.Warn("index file", "err", err)
		}
		done++
		idx.setProgress(total, done)
		return nil
	})
	if err != nil {
		return err
	}
	for rel, rec := range known {
		if _, ok := seenFiles[rel]; ok {
			continue
		}
		current, err := idx.store.GetByID(ctx, rec.ID)
		if err == nil && current.Available && current.RelativePath != rel {
			continue
		}
		if err := idx.store.MarkMissing(ctx, rel); err != nil {
			return err
		}
		if rec.Available {
			rec.Available = false
			idx.emit(ctx, ChangeDelete, rec)
		}
	}
	idx.setProgress(total, total)
	return nil
}

func (idx *Indexer) IndexPath(ctx context.Context, abs string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	root, err := files.CleanShareRoot(idx.root())
	if err != nil {
		return err
	}
	rel, err := files.RelFromRoot(root, abs)
	if err != nil {
		return err
	}
	return idx.indexFileLocked(ctx, root, rel, abs)
}

func (idx *Indexer) RemovePath(ctx context.Context, abs string) error {
	root := idx.root()
	rel, err := files.RelFromRoot(filepath.Clean(root), filepath.Clean(abs))
	if err != nil {
		// Fall back to treating abs as already-relative.
		rel = filepath.ToSlash(abs)
	}
	rec, recErr := idx.store.GetByPath(ctx, rel)
	if err := idx.store.MarkMissing(ctx, rel); err != nil {
		return err
	}
	if recErr == nil && rec.Available {
		rec.Available = false
		idx.emit(ctx, ChangeDelete, rec)
	}
	return idx.store.MarkDirMissing(ctx, rel)
}

func (idx *Indexer) indexFileLocked(ctx context.Context, root, rel, abs string) error {
	resolved, err := files.ResolveUnderRoot(root, rel)
	if err != nil {
		return err
	}
	info, err := waitStable(resolved)
	if err != nil {
		return err
	}
	sum, err := files.HashFile(resolved)
	if err != nil {
		return err
	}
	prev, _ := idx.store.GetByPath(ctx, rel)
	if cand, ok := idx.moveCandidate(ctx, root, rel, sum); ok {
		if prev.ID != "" && prev.ID != cand.ID {
			_ = idx.store.DeleteByID(ctx, prev.ID)
		}
		prev = cand
	}
	vis := files.VisibilityPublic
	if prev.Visibility != "" {
		vis = prev.Visibility
	}
	rec := files.Record{
		ID:           prev.ID,
		RelativePath: rel,
		Filename:     filepath.Base(rel),
		MIMEType:     files.DetectMIME(resolved),
		Size:         info.Size(),
		Hash:         sum,
		ModifiedAt:   info.ModTime().UTC(),
		Available:    true,
		Visibility:   vis,
	}
	var saved files.Record
	if rec.ID != "" {
		saved, err = idx.store.Save(ctx, rec)
	} else {
		saved, err = idx.store.Upsert(ctx, rec)
	}
	if err != nil {
		return err
	}
	if prev.ID != "" && prev.Available && prev.Hash == saved.Hash && prev.Size == saved.Size && prev.RelativePath == saved.RelativePath {
		return nil
	}
	kind := ChangeCreate
	if prev.ID != "" {
		kind = ChangeUpdate
	}
	idx.emit(ctx, kind, saved)
	return nil
}

func (idx *Indexer) moveCandidate(ctx context.Context, root, rel string, sum files.ContentID) (files.Record, bool) {
	matches, err := idx.store.ListByHash(ctx, sum.Algorithm, sum.Digest)
	if err != nil || len(matches) == 0 {
		return files.Record{}, false
	}
	var samePath, missing, stale files.Record
	for _, rec := range matches {
		if rec.RelativePath == rel {
			samePath = rec
			continue
		}
		if !rec.Available {
			if missing.ID == "" {
				missing = rec
			}
			continue
		}
		abs, err := files.ResolveUnderRoot(root, rec.RelativePath)
		if err != nil {
			if stale.ID == "" {
				stale = rec
			}
			continue
		}
		if _, err := os.Lstat(abs); err != nil && stale.ID == "" {
			stale = rec
		}
	}
	if missing.ID != "" {
		return missing, true
	}
	if stale.ID != "" {
		return stale, true
	}
	if samePath.ID != "" {
		return samePath, true
	}
	return files.Record{}, false
}

func waitStable(path string) (os.FileInfo, error) {
	var prev os.FileInfo
	for i := 0; i < maxStableTry; i++ {
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, files.ErrSymlink
		}
		if time.Since(info.ModTime()) > time.Second {
			return info, nil
		}
		if prev != nil && prev.Size() == info.Size() && prev.ModTime().Equal(info.ModTime()) {
			return info, nil
		}
		prev = info
		time.Sleep(stabilityWait)
	}
	return prev, nil
}

func (idx *Indexer) setProgress(total, done int) {
	if idx.status == nil {
		return
	}
	_ = idx.status.Update(func(snap *status.Snapshot) error {
		snap.IndexingTotal = total
		snap.IndexingDone = done
		return nil
	})
}

func (idx *Indexer) PublishSummary(ctx context.Context) {
	if idx.status == nil {
		return
	}
	sum, err := idx.store.Summary(ctx)
	if err != nil {
		return
	}
	_ = idx.status.Update(func(snap *status.Snapshot) error {
		snap.IndexedFiles = sum.Files
		snap.TotalBytes = sum.Bytes
		snap.IndexingDone = sum.Files
		snap.IndexingTotal = sum.Files
		return nil
	})
}
