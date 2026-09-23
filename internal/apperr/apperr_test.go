package apperr

import (
	"errors"
	"testing"
)

func TestWrapAndClassify(t *testing.T) {
	root := errors.New("disk full")
	err := Wrap(KindDatabase, "FediShare could not open its database.", root)

	if Classify(err) != KindDatabase {
		t.Fatalf("kind = %s", Classify(err))
	}
	if UserMessage(err) != "FediShare could not open its database." {
		t.Fatalf("user message = %q", UserMessage(err))
	}
	if !errors.Is(err, root) {
		t.Fatal("expected unwrap to the root cause")
	}
}

func TestUserMessageUnknown(t *testing.T) {
	if got := UserMessage(errors.New("secret=/tmp/key.pem")); got != "Something went wrong." {
		t.Fatalf("raw errors must not leak to the UI, got %q", got)
	}
}

func TestKindString(t *testing.T) {
	if KindFilesystem.String() != "filesystem" {
		t.Fatalf("got %s", KindFilesystem)
	}
}
