// Package apperr classifies errors so the UI can show a short message
// while logs keep the technical cause.
package apperr

import (
	"errors"
	"fmt"
)

// Kind is a stable category the status service and future UI can switch on.
type Kind int

const (
	KindUnknown Kind = iota
	KindTemporaryNetwork
	KindInvalidConfig
	KindFilesystem
	KindDatabase
	KindFederationReject
	KindCrypto
)

func (k Kind) String() string {
	switch k {
	case KindTemporaryNetwork:
		return "temporary_network"
	case KindInvalidConfig:
		return "invalid_config"
	case KindFilesystem:
		return "filesystem"
	case KindDatabase:
		return "database"
	case KindFederationReject:
		return "federation_reject"
	case KindCrypto:
		return "crypto"
	default:
		return "unknown"
	}
}

// Error is a classified application error.
type Error struct {
	Kind Kind
	Msg  string // short, non-technical, safe to show in the UI
	Err  error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Msg, e.Err)
	}
	return e.Msg
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// UserMessage returns a message suitable for the tray or dashboard.
func (e *Error) UserMessage() string {
	if e == nil || e.Msg == "" {
		return "Something went wrong."
	}
	return e.Msg
}

func Wrap(kind Kind, userMsg string, err error) error {
	if err == nil {
		return nil
	}
	var existing *Error
	if errors.As(err, &existing) {
		if userMsg == "" {
			userMsg = existing.Msg
		}
		return &Error{Kind: kind, Msg: userMsg, Err: err}
	}
	return &Error{Kind: kind, Msg: userMsg, Err: err}
}

func New(kind Kind, userMsg string) error {
	return &Error{Kind: kind, Msg: userMsg}
}

// Classify returns the Kind of err, or KindUnknown.
func Classify(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindUnknown
}

// UserMessage extracts a UI-safe message when possible.
func UserMessage(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.UserMessage()
	}
	if err == nil {
		return ""
	}
	return "Something went wrong."
}
