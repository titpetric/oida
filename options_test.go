package oida

import "testing"

func TestOptionsDefaultsAndValidation(t *testing.T) {
	opts := Options{}.WithDefaults()
	if opts.Path != DefaultPath || opts.RingBufferSize != 200 || opts.SampleRate != 1 || opts.Clock == nil {
		t.Fatalf("unexpected defaults: %+v", opts)
	}
	if err := opts.Validate(); err != nil {
		t.Fatalf("defaults are invalid: %v", err)
	}

	trimmed := Options{Path: "/debug/oida/"}.WithDefaults()
	if trimmed.Path != DefaultPath {
		t.Fatalf("path is %q, want the trailing slash trimmed", trimmed.Path)
	}

	zeroes := NewOptions()
	zeroes.RingBufferSize = 0
	zeroes.TopRequests = 0
	zeroes.MaxSpansPerTrace = 0
	zeroes.SampleRate = 0
	zeroes.RefreshInterval = 0
	zeroes = zeroes.WithDefaults()
	if zeroes.RingBufferSize != 0 || zeroes.TopRequests != 0 || zeroes.MaxSpansPerTrace != 0 ||
		zeroes.SampleRate != 0 || zeroes.RefreshInterval != 0 {
		t.Fatalf("explicit zero values were replaced: %+v", zeroes)
	}

	if !opts.ignored(DefaultPath + "/stats") {
		t.Error("the front end path is traced")
	}
	if !opts.ignored("/healthz") || opts.ignored("/users/1") {
		t.Error("ignore paths do not match as documented")
	}

	prefixed := NewOptions()
	prefixed.IgnorePaths = []string{"/assets/*"}
	if !prefixed.ignored("/assets/app.css") || prefixed.ignored("/assetsx") {
		t.Error("prefix ignore patterns do not match as documented")
	}
}
