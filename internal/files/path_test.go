package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("no"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ResolveUnderRoot(root, "../"+filepath.Base(outside)+"/secret.txt"); err == nil {
		t.Fatal("parent traversal")
	}
	if _, err := ResolveUnderRoot(root, "..\\secret.txt"); err == nil {
		t.Fatal("backslash traversal")
	}
	if _, err := ResolveUnderRoot(root, "foo/../../etc/passwd"); err == nil {
		t.Fatal("encoded traversal")
	}
	if _, err := ResolveUnderRoot(root, "/etc/passwd"); err == nil {
		t.Fatal("absolute")
	}
	got, err := ResolveUnderRoot(root, "ok.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(root, "ok.txt") {
		t.Fatalf("got %s", got)
	}
}

func TestResolveRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("no"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Skip("symlinks not supported")
	}
	if _, err := ResolveUnderRoot(root, "link.txt"); err == nil {
		t.Fatal("symlink file")
	}
	if err := os.Symlink(outside, filepath.Join(root, "other")); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveUnderRoot(root, "other/secret.txt"); err == nil {
		t.Fatal("symlink dir")
	}
}

func TestCleanShareRootRejectsSymlink(t *testing.T) {
	realDir := t.TempDir()
	link := filepath.Join(t.TempDir(), "root")
	if err := os.Symlink(realDir, link); err != nil {
		t.Skip("symlinks not supported")
	}
	if _, err := CleanShareRoot(link); err == nil {
		t.Fatal("symlink root")
	}
}

func TestRelFromRootHidden(t *testing.T) {
	root := t.TempDir()
	if _, err := RelFromRoot(root, filepath.Join(root, ".ssh", "id_rsa")); err != ErrHidden {
		t.Fatalf("err=%v", err)
	}
}
