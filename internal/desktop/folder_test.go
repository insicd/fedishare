package desktop

import (
	"path/filepath"
	"testing"
)

func TestCleanFolderPath(t *testing.T) {
	got, err := CleanFolderPath("  /Users/alice/FediShare/  \n")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/Users/alice/FediShare" {
		t.Fatalf("got %q", got)
	}
	quoted, err := CleanFolderPath(`"/Users/alice/FediShare"`)
	if err != nil {
		t.Fatal(err)
	}
	if quoted != "/Users/alice/FediShare" {
		t.Fatalf("quoted: %q", quoted)
	}
	if _, err := CleanFolderPath("   "); err != ErrCanceled {
		t.Fatalf("empty: %v", err)
	}
	rel, err := CleanFolderPath("share")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(rel) {
		t.Fatalf("relative stayed relative: %q", rel)
	}
}
