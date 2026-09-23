package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type LookupFunc func(ctx context.Context, id string) (Record, error)

type DownloadOptions struct {
	Root    func() string
	Lookup  LookupFunc
	Paused  func() bool
	Limiter *Limiter
}

func DownloadHandler(opts DownloadOptions) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		if id == "" || !ValidOpaqueID(id) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if opts.Paused != nil && opts.Paused() {
			http.Error(w, "sharing is paused", http.StatusServiceUnavailable)
			return
		}
		rec, err := opts.Lookup(r.Context(), id)
		if err != nil || !rec.Available {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if rec.Visibility != "" && rec.Visibility != VisibilityPublic {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		root := ""
		if opts.Root != nil {
			root = opts.Root()
		}
		abs, err := ResolveUnderRoot(root, rec.RelativePath)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		info, err := os.Lstat(abs)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err := stillInside(root, abs); err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		f, err := openNoFollow(abs)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		defer f.Close()

		if opts.Limiter != nil {
			release, err := opts.Limiter.Acquire(r)
			if err != nil {
				http.Error(w, "too many downloads", http.StatusTooManyRequests)
				return
			}
			defer release()
		}

		mod := info.ModTime().UTC()
		etag := `"` + rec.Hash.Digest + `"`
		w.Header().Set("Content-Type", rec.MIMEType)
		w.Header().Set("Content-Disposition", contentDisposition(rec.Filename))
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", etag)
		w.Header().Set("Last-Modified", mod.Format(http.TimeFormat))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")

		if match := r.Header.Get("If-None-Match"); match != "" && etagWeakMatch(match, etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if t, err := http.ParseTime(r.Header.Get("If-Modified-Since")); err == nil && !mod.After(t.Add(time.Second)) {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		size := info.Size()
		ranges, err := parseRange(r.Header.Get("Range"), size)
		if err != nil {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
			http.Error(w, "invalid range", http.StatusRequestedRangeNotSatisfiable)
			return
		}

		start, end := int64(0), size-1
		code := http.StatusOK
		if len(ranges) == 1 {
			start, end = ranges[0].start, ranges[0].end
			code = http.StatusPartialContent
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
		}
		length := end - start + 1
		if length < 0 {
			length = 0
		}
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))

		if r.Method == http.MethodHead {
			w.WriteHeader(code)
			return
		}
		if _, err := f.Seek(start, io.SeekStart); err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(code)
		var src io.Reader = io.LimitReader(f, length)
		if opts.Limiter != nil {
			src = LimitReader(src, opts.Limiter.Limits.BandwidthBPS)
		}
		_, _ = io.Copy(w, src)
	})
}

// ValidOpaqueID reports whether id is a hex identifier, never a path.
func ValidOpaqueID(id string) bool {
	if len(id) < 16 || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !unicode.Is(unicode.ASCII_Hex_Digit, r) {
			return false
		}
	}
	return true
}

func contentDisposition(name string) string {
	safe := strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	if safe == "" {
		safe = "download"
	}
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, safe, url.PathEscape(name))
}

func etagWeakMatch(header, etag string) bool {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		part = strings.TrimPrefix(part, "W/")
		if part == etag || part == "*" {
			return true
		}
	}
	return false
}

type byteRange struct {
	start, end int64
}

func parseRange(header string, size int64) ([]byteRange, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil, nil
	}
	if size <= 0 {
		return nil, errors.New("empty")
	}
	if !strings.HasPrefix(header, "bytes=") {
		return nil, errors.New("unit")
	}
	spec := strings.TrimPrefix(header, "bytes=")
	// One range is enough for segmented downloads later.
	part := strings.TrimSpace(strings.Split(spec, ",")[0])
	if part == "" {
		return nil, errors.New("empty")
	}
	se := strings.SplitN(part, "-", 2)
	if len(se) != 2 {
		return nil, errors.New("dash")
	}
	var start, end int64
	switch {
	case se[0] == "" && se[1] != "":
		suffix, err := strconv.ParseInt(se[1], 10, 64)
		if err != nil || suffix <= 0 {
			return nil, err
		}
		if suffix > size {
			suffix = size
		}
		start = size - suffix
		end = size - 1
	case se[0] != "" && se[1] == "":
		var err error
		start, err = strconv.ParseInt(se[0], 10, 64)
		if err != nil || start < 0 || start >= size {
			return nil, errors.New("start")
		}
		end = size - 1
	default:
		var err error
		start, err = strconv.ParseInt(se[0], 10, 64)
		if err != nil {
			return nil, err
		}
		end, err = strconv.ParseInt(se[1], 10, 64)
		if err != nil {
			return nil, err
		}
		if start < 0 || end < start || start >= size {
			return nil, errors.New("bounds")
		}
		if end >= size {
			end = size - 1
		}
	}
	return []byteRange{{start: start, end: end}}, nil
}
