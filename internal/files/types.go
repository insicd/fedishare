package files

import (
	"fmt"
	"time"
)

const (
	AlgoSHA256 = "sha256"

	VisibilityPublic         = "public"
	VisibilityFollowers      = "followers"
	VisibilitySelectedActors = "selected"
	VisibilityPrivate        = "private"
)

// ContentID is algorithm + digest so a later hash (for example BLAKE3)
// can be introduced without rewriting callers.
type ContentID struct {
	Algorithm string `json:"algorithm"`
	Digest    string `json:"digest"`
}

func (c ContentID) URN() string {
	if c.Algorithm == "" || c.Digest == "" {
		return ""
	}
	return fmt.Sprintf("fedishare:%s:%s", c.Algorithm, c.Digest)
}

func (c ContentID) String() string { return c.URN() }

// Record is one indexed file. ID is an opaque identifier, never a path.
type Record struct {
	ID           string    `json:"id"`
	RelativePath string    `json:"relative_path"`
	Filename     string    `json:"name"`
	MIMEType     string    `json:"mime_type"`
	Size         int64     `json:"size"`
	Hash         ContentID `json:"hash"`
	ModifiedAt   time.Time `json:"modified_at"`
	IndexedAt    time.Time `json:"indexed_at"`
	Available    bool      `json:"available"`
	Visibility   string    `json:"visibility"`
}

// Entry is the dashboard/API view of a record.
type Entry struct {
	ID           string `json:"id"`
	RelativePath string `json:"relative_path"`
	Name         string `json:"name"`
	MIMEType     string `json:"mime_type"`
	Size         int64  `json:"size"`
	HashURN      string `json:"hash"`
	Available    bool   `json:"available"`
	DownloadURL  string `json:"download_url,omitempty"`
}

func (r Record) Entry() Entry {
	return Entry{
		ID:           r.ID,
		RelativePath: r.RelativePath,
		Name:         r.Filename,
		MIMEType:     r.MIMEType,
		Size:         r.Size,
		HashURN:      r.Hash.URN(),
		Available:    r.Available,
		DownloadURL:  "/files/" + r.ID,
	}
}

type Summary struct {
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`
}

type DirRecord struct {
	ID           string
	RelativePath string
	Name         string
	IndexedAt    time.Time
	Available    bool
}
