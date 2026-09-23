package indexer

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fedishare/fedishare/internal/files"
	"github.com/fsnotify/fsnotify"
)

const debounce = 400 * time.Millisecond

type Watcher struct {
	idx *Indexer
	log *slog.Logger

	mu      sync.Mutex
	pending map[string]*time.Timer
}

func NewWatcher(idx *Indexer, log *slog.Logger) *Watcher {
	if log == nil {
		log = slog.Default()
	}
	return &Watcher{idx: idx, log: log, pending: make(map[string]*time.Timer)}
}

func (w *Watcher) Run(ctx context.Context) error {
	root, err := files.CleanShareRoot(w.idx.root())
	if err != nil {
		return err
	}
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer fw.Close()

	if err := addTree(fw, root); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			w.flushCancel()
			return ctx.Err()
		case ev, ok := <-fw.Events:
			if !ok {
				return nil
			}
			w.handle(ctx, fw, ev)
		case err, ok := <-fw.Errors:
			if !ok {
				return nil
			}
			w.log.Warn("filesystem watch", "err", err)
		}
	}
}

func (w *Watcher) handle(ctx context.Context, fw *fsnotify.Watcher, ev fsnotify.Event) {
	name := ev.Name
	if ev.Has(fsnotify.Create) {
		info, err := os.Lstat(name)
		if err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 && !files.IsDotfile(info.Name()) {
			_ = fw.Add(name)
		}
	}
	if ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename) {
		w.mu.Lock()
		if t, ok := w.pending[name]; ok {
			t.Stop()
			delete(w.pending, name)
		}
		w.mu.Unlock()
		go func() {
			if err := w.idx.RemovePath(ctx, name); err != nil {
				w.log.Debug("index remove", "err", err)
			}
			w.idx.PublishSummary(ctx)
		}()
		return
	}
	if ev.Has(fsnotify.Create) || ev.Has(fsnotify.Write) || ev.Has(fsnotify.Chmod) {
		w.schedule(ctx, name)
	}
}

func (w *Watcher) schedule(ctx context.Context, name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if t, ok := w.pending[name]; ok {
		t.Stop()
	}
	w.pending[name] = time.AfterFunc(debounce, func() {
		w.mu.Lock()
		delete(w.pending, name)
		w.mu.Unlock()
		info, err := os.Lstat(name)
		if err != nil {
			_ = w.idx.RemovePath(ctx, name)
			w.idx.PublishSummary(ctx)
			return
		}
		if info.IsDir() {
			return
		}
		if err := w.idx.IndexPath(ctx, name); err != nil {
			w.log.Debug("index update", "err", err)
		}
		w.idx.PublishSummary(ctx)
	})
}

func (w *Watcher) flushCancel() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, t := range w.pending {
		t.Stop()
	}
	w.pending = make(map[string]*time.Timer)
}

func addTree(fw *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if files.IsDotfile(d.Name()) && path != root {
			return fs.SkipDir
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fs.SkipDir
		}
		return fw.Add(path)
	})
}
