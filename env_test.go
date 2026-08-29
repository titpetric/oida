package oida

import (
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestEnvAuth(t *testing.T) {
	t.Setenv("OIDA_AUTH", "admin:hunter2")
	t.Setenv("OIDA_SIGNING_SECRET", "pre-shared")

	tracer, err := Configure(NewOptions("env-test"))
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}

	opts := tracer.Options()
	if opts.SigningSecret != "pre-shared" {
		t.Errorf("signing secret is %q, want the environment's", opts.SigningSecret)
	}
	hash, ok := opts.Users["admin"]
	if !ok {
		t.Fatalf("users are %v, want admin from OIDA_AUTH", opts.Users)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("hunter2")); err != nil {
		t.Errorf("the stored hash does not verify the password: %v", err)
	}
}

func TestEnvOptions(t *testing.T) {
	t.Setenv("OIDA_PATH", "/internal/oida")
	t.Setenv("OIDA_RING_BUFFER_SIZE", "50")
	t.Setenv("OIDA_SAMPLE_RATE", "25")
	t.Setenv("OIDA_TRACK_MEMORY_USE", "false")
	t.Setenv("OIDA_IGNORE_PATHS", "/ping, /pong")
	t.Setenv("OIDA_ALLOWED_NETWORKS", "127.0.0.0/8,10.0.0.0/8")

	tracer, err := Configure(NewOptions("env-test"))
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}

	opts := tracer.Options()
	if opts.Path != "/internal/oida" {
		t.Errorf("path is %q", opts.Path)
	}
	if opts.RingBufferSize != 50 {
		t.Errorf("ring buffer size is %d", opts.RingBufferSize)
	}
	if opts.SampleRate != 25 {
		t.Errorf("sample rate is %v", opts.SampleRate)
	}
	if opts.TrackMemoryUse {
		t.Error("memory tracking stayed on")
	}
	if len(opts.IgnorePaths) != 2 || opts.IgnorePaths[0] != "/ping" || opts.IgnorePaths[1] != "/pong" {
		t.Errorf("ignore paths are %v", opts.IgnorePaths)
	}
	if len(opts.AllowedNetworks) != 2 {
		t.Errorf("allowed networks are %v", opts.AllowedNetworks)
	}
}

func TestEnvKeepsConfigured(t *testing.T) {
	t.Setenv("OIDA_AUTH", "admin:hunter2")
	t.Setenv("OIDA_SIGNING_SECRET", "environment")
	t.Setenv("OIDA_RING_BUFFER_SIZE", "50")

	opts := NewOptions("env-test")
	opts.Users = map[string]string{"operator": "$2b$05$configured"}
	opts.SigningSecret = "configured"
	opts.RingBufferSize = 500

	tracer, err := Configure(opts)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}

	got := tracer.Options()
	if _, ok := got.Users["admin"]; ok {
		t.Error("the environment user overrode the configured users")
	}
	if got.SigningSecret != "configured" {
		t.Errorf("signing secret is %q, want the configured one", got.SigningSecret)
	}
	if got.RingBufferSize != 500 {
		t.Errorf("ring buffer size is %d, want the configured 500", got.RingBufferSize)
	}
}

func TestEnvRejectsBadValues(t *testing.T) {
	t.Setenv("OIDA_AUTH", "no-separator")

	if _, err := Configure(NewOptions("env-test")); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err is %v, want ErrInvalidOptions for OIDA_AUTH", err)
	}

	t.Setenv("OIDA_AUTH", "")
	t.Setenv("OIDA_RING_BUFFER_SIZE", "many")

	if _, err := Configure(NewOptions("env-test")); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err is %v, want ErrInvalidOptions for OIDA_RING_BUFFER_SIZE", err)
	}
}

func TestEnvClampsSampleRate(t *testing.T) {
	t.Setenv("OIDA_SAMPLE_RATE", "250")

	tracer, err := Configure(NewOptions("env-test"))
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if rate := tracer.Options().SampleRate; rate != 100 {
		t.Errorf("sample rate is %v, want 250 clamped to 100", rate)
	}

	t.Setenv("OIDA_SAMPLE_RATE", "-3")

	tracer, err = Configure(NewOptions("env-test"))
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if rate := tracer.Options().SampleRate; rate != 0 {
		t.Errorf("sample rate is %v, want -3 clamped to 0", rate)
	}
}
