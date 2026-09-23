package files

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestHashReaderKnownVector(t *testing.T) {
	// SHA-256("hello")
	const want = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	got, err := HashReader(bytes.NewReader([]byte("hello")))
	if err != nil {
		t.Fatal(err)
	}
	if got.Algorithm != AlgoSHA256 || got.Digest != want {
		t.Fatalf("got %+v", got)
	}
	if got.URN() != "fedishare:sha256:"+want {
		t.Fatalf("urn %s", got.URN())
	}
}

func TestHashFileStreams(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.bin")
	data := bytes.Repeat([]byte("abcdefg\n"), 8000)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	fromFile, err := HashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fromMem, err := HashReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if fromFile != fromMem {
		t.Fatalf("%+v vs %+v", fromFile, fromMem)
	}
}
