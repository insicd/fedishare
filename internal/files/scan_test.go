package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectCountsRegularFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "b.bin"), []byte("world!"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".hidden"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}

	sum, list, err := Inspect(root, 10)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Files != 2 || sum.Bytes != 11 {
		t.Fatalf("sum = %+v", sum)
	}
	if len(list) != 2 {
		t.Fatalf("list = %+v", list)
	}
}

func TestInspectRejectsSymlinkRoot(t *testing.T) {
	realDir := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Skip("symlinks not supported")
	}
	if _, _, err := Inspect(link, 10); err == nil {
		t.Fatal("expected symlink root to be rejected")
	}
}

func TestInspectSkipsSymlinkFileAndDir(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "escape.txt")); err != nil {
		t.Skip("symlinks not supported")
	}
	if err := os.Symlink(outside, filepath.Join(root, "otherdir")); err != nil {
		t.Fatal(err)
	}

	sum, list, err := Inspect(root, 10)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Files != 1 || len(list) != 1 || list[0].Name != "ok.txt" {
		t.Fatalf("symlink contents leaked: sum=%+v list=%+v", sum, list)
	}
}

func TestInspectRequiresAbsolute(t *testing.T) {
	if _, _, err := Inspect("relative", 1); err == nil {
		t.Fatal("expected error")
	}
}
