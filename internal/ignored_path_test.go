package internal

import (
	"testing"

	"github.com/titpetric/oida/model"
)

func TestOptionsDefaultsAndValidation(t *testing.T) {
	opts := model.Options{}.WithDefaults()
	if opts.Path != model.DefaultPath || opts.RingBufferSize != 200 || opts.SampleRate != 100 || opts.Clock == nil {
		t.Fatalf("unexpected defaults: %+v", opts)
	}
	if err := opts.Validate(); err != nil {
		t.Fatalf("defaults are invalid: %v", err)
	}

	trimmed := model.Options{Path: "/debug/oida/"}.WithDefaults()
	if trimmed.Path != model.DefaultPath {
		t.Fatalf("path is %q, want the trailing slash trimmed", trimmed.Path)
	}

	zeroes := model.NewOptions("")
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

	if !IgnoredPath(opts, model.DefaultPath+"/stats") {
		t.Error("the front end path is traced")
	}
	if !IgnoredPath(opts, "/healthz") || IgnoredPath(opts, "/users/1") {
		t.Error("ignore paths do not match as documented")
	}

	prefixed := model.NewOptions("")
	prefixed.IgnorePaths = []string{"/assets/*"}
	if !IgnoredPath(prefixed, "/assets/app.css") || IgnoredPath(prefixed, "/assetsx") {
		t.Error("prefix ignore patterns do not match as documented")
	}
}
