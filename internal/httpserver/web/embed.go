package web

import "embed"

//go:embed templates/*.html static/*
var Templates embed.FS

// Static is the same filesystem; callers Sub() the static directory.
var Static = Templates
