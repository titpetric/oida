package oida

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestEnvAuth(t *testing.T) {
	t.Setenv("OIDA_AUTH", "admin:hunter2")
	t.Setenv("OIDA_SIGNING_SECRET", "pre-shared")

	tracer, err := New(NewOptions("env-test"))
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

	tracer, err := New(NewOptions("env-test"))
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

	tracer, err := New(opts)
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

	if _, err := New(NewOptions("env-test")); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err is %v, want ErrInvalidOptions for OIDA_AUTH", err)
	}

	t.Setenv("OIDA_AUTH", "")
	t.Setenv("OIDA_RING_BUFFER_SIZE", "many")

	if _, err := New(NewOptions("env-test")); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err is %v, want ErrInvalidOptions for OIDA_RING_BUFFER_SIZE", err)
	}
}

func TestEnvClampsSampleRate(t *testing.T) {
	t.Setenv("OIDA_SAMPLE_RATE", "250")

	tracer, err := New(NewOptions("env-test"))
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if rate := tracer.Options().SampleRate; rate != 100 {
		t.Errorf("sample rate is %v, want 250 clamped to 100", rate)
	}

	t.Setenv("OIDA_SAMPLE_RATE", "-3")

	tracer, err = New(NewOptions("env-test"))
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if rate := tracer.Options().SampleRate; rate != 0 {
		t.Errorf("sample rate is %v, want -3 clamped to 0", rate)
	}
}

// envStorage builds the retention driver the environment asks for, which is
// what the tracer would hold. Options.Storage is not readable back off a
// tracer, so the env layer is exercised where it runs.
func envStorage(t *testing.T) Storage {
	t.Helper()

	opts := NewOptions("env-test")
	if err := optionsFromEnv(&opts); err != nil {
		t.Fatalf("optionsFromEnv: %v", err)
	}
	if opts.Storage == nil {
		// A nil driver is the memory default the tracer builds itself.
		store, err := newStorageMemory(opts.RingBufferSize)
		if err != nil {
			t.Fatalf("newStorageMemory: %v", err)
		}
		return store
	}
	return opts.Storage
}

// storesDocument saves a trace through store and reports whether it landed in
// dir as a document, which is what tells the disk driver from the memory one.
func storesDocument(t *testing.T, store Storage, dir, id string) bool {
	t.Helper()

	if err := store.Save(context.Background(), Trace{ID: id, Name: "probe"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	name := filepath.Join(dir, id+".json")
	t.Cleanup(func() { _ = os.Remove(name) })
	_, err := os.Stat(name)
	return err == nil
}

func TestEnvStorageDiskPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OIDA_STORAGE_DISK_PATH", dir)

	if !storesDocument(t, envStorage(t), dir, "01ARZ3NDEKTSV4RRFFQ69G5FAV") {
		t.Fatal("the configured storage wrote no document, want the disk driver")
	}

	// A path that cannot exist fails Configure; the error says why.
	t.Setenv("OIDA_STORAGE_DISK_PATH", "/dev/null/never")
	if _, err := New(NewOptions("env-test")); err == nil {
		t.Fatal("Configure accepted an impossible storage path")
	}
}

func TestEnvStorageDriver(t *testing.T) {
	// The disk driver needs no path: it falls back to a folder under the
	// temporary directory, which is where its documents then land.
	fallback := filepath.Join(os.TempDir(), "oida-traces")
	t.Setenv("OIDA_STORAGE_DRIVER", "disk")

	if !storesDocument(t, envStorage(t), fallback, "01ARZ3NDEKTSV4RRFFQ69G5FAV") {
		t.Fatalf("the disk driver wrote no document under %s", fallback)
	}

	// The memory driver is the default, and is what an explicit name gets. It
	// writes nowhere.
	t.Setenv("OIDA_STORAGE_DRIVER", "memory")
	if storesDocument(t, envStorage(t), fallback, "01ARZ3NDEKTSV4RRFFQ69G5FAW") {
		t.Fatal("the memory driver wrote a document")
	}
}

func TestEnvStorageDriverRejects(t *testing.T) {
	t.Setenv("OIDA_STORAGE_DRIVER", "postgres")
	if _, err := New(NewOptions("env-test")); err == nil {
		t.Fatal("Configure accepted an unknown storage driver")
	}

	// Settings the named driver cannot honour are a contradiction, not a hint.
	t.Setenv("OIDA_STORAGE_DRIVER", "memory")
	t.Setenv("OIDA_STORAGE_DISK_PATH", t.TempDir())
	if _, err := New(NewOptions("env-test")); err == nil {
		t.Fatal("Configure accepted a disk path for the memory driver")
	}
}

func TestEnvStorageSizes(t *testing.T) {
	// Each driver takes its size from the variable carrying its name.
	t.Setenv("OIDA_STORAGE_MEMORY_SIZE", "7")

	if size := envStorage(t).Cap(); size != 7 {
		t.Errorf("memory driver holds %d traces, want 7", size)
	}

	t.Setenv("OIDA_STORAGE_DISK_PATH", t.TempDir())
	t.Setenv("OIDA_STORAGE_DISK_LIMIT", "9")
	t.Setenv("OIDA_STORAGE_MEMORY_SIZE", "")

	if size := envStorage(t).Cap(); size != 9 {
		t.Errorf("disk driver holds %d documents, want 9", size)
	}

	// A size that is not a number fails rather than falling back.
	t.Setenv("OIDA_STORAGE_DISK_LIMIT", "many")
	if _, err := New(NewOptions("env-test")); err == nil {
		t.Fatal("Configure accepted a non numeric disk limit")
	}
}

func TestEnvStorageDiskList(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	t.Setenv("OIDA_STORAGE_DISK_PATH", dir)
	if err := envStorage(t).Save(ctx, Trace{ID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", Name: "seed"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Off by default: the folder holds a document the ring does not list.
	if length, _ := envStorage(t).Len(ctx); length != 0 {
		t.Fatalf("the ring holds %d traces without OIDA_STORAGE_DISK_LIST", length)
	}

	// On, the folder is read into the ring once, when the driver is built.
	t.Setenv("OIDA_STORAGE_DISK_LIST", "true")
	traces, err := envStorage(t).List(ctx, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(traces) != 1 || traces[0].Name != "seed" {
		t.Fatalf("the ring holds %v, want the stored trace", traces)
	}
}

func TestEnvStorageDiskExpire(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	t.Setenv("OIDA_STORAGE_DISK_PATH", dir)
	if err := envStorage(t).Save(ctx, Trace{ID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", Name: "old"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// The folder is aged out once, before it is read back, so an expired
	// document is neither on disk nor in the ring.
	t.Setenv("OIDA_STORAGE_DISK_EXPIRE", "1ns")
	t.Setenv("OIDA_STORAGE_DISK_LIST", "true")
	if length, _ := envStorage(t).Len(ctx); length != 0 {
		t.Errorf("the ring holds %d traces, want the expired document gone", length)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("the folder holds %d documents, want none", len(entries))
	}

	// A value that is not a positive duration fails rather than being read
	// as one.
	for _, value := range []string{"soon", "0", "-1h"} {
		t.Setenv("OIDA_STORAGE_DISK_EXPIRE", value)
		if _, err := New(NewOptions("env-test")); !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("OIDA_STORAGE_DISK_EXPIRE=%q gave %v, want ErrInvalidOptions", value, err)
		}
	}
}

// envKeys is every variable optionsFromEnv reads, which is what the empty and
// whitespace cases below are set to.
var envKeys = []string{
	"OIDA_SERVICE_NAME", "OIDA_PATH", "OIDA_ENABLED", "OIDA_RING_BUFFER_SIZE",
	"OIDA_TOP_REQUESTS", "OIDA_MAX_SPANS_PER_TRACE", "OIDA_SAMPLE_RATE",
	"OIDA_TRACK_MEMORY_USE", "OIDA_TRUST_REQUEST_ID", "OIDA_REFRESH_INTERVAL",
	"OIDA_LIVE_STREAM", "OIDA_CAPTURE_LOGS", "OIDA_IGNORE_PATHS",
	"OIDA_ALLOWED_NETWORKS", "OIDA_STORAGE_DRIVER", "OIDA_STORAGE_MEMORY_SIZE",
	"OIDA_STORAGE_DISK_PATH", "OIDA_STORAGE_DISK_LIMIT", "OIDA_STORAGE_DISK_LIST",
	"OIDA_STORAGE_DISK_EXPIRE", "OIDA_AUTH", "OIDA_USERS_FILE",
	"OIDA_SIGNING_SECRET",
}

func TestEnvSetToNothingKeepsEveryDefault(t *testing.T) {
	// A variable set to nothing is a variable unset: compose writes an empty
	// value for a setting left alone, and a deployment that exports one should
	// get the default rather than the zero value.
	for _, value := range []string{"", "   ", "\t"} {
		for _, key := range envKeys {
			t.Setenv(key, value)
		}

		tracer, err := New(NewOptions("env-test"))
		if err != nil {
			t.Fatalf("Configure with every variable set to %q: %v", value, err)
		}

		got, want := tracer.Options(), NewOptions("env-test").WithDefaults()
		if got.ServiceName != want.ServiceName || got.Path != want.Path {
			t.Errorf("%q: name %q path %q, want %q and %q", value, got.ServiceName, got.Path, want.ServiceName, want.Path)
		}
		if got.Enabled != want.Enabled || got.TrackMemoryUse != want.TrackMemoryUse ||
			got.TrustRequestID != want.TrustRequestID || got.LiveStream != want.LiveStream ||
			got.CaptureLogs != want.CaptureLogs {
			t.Errorf("%q: booleans are %+v, want %+v", value, got, want)
		}
		if got.RingBufferSize != want.RingBufferSize || got.TopRequests != want.TopRequests ||
			got.MaxSpansPerTrace != want.MaxSpansPerTrace || got.RefreshInterval != want.RefreshInterval ||
			got.SampleRate != want.SampleRate {
			t.Errorf("%q: sizes are %+v, want %+v", value, got, want)
		}
		if !slices.Equal(got.IgnorePaths, want.IgnorePaths) {
			t.Errorf("%q: ignore paths are %v, want the default %v", value, got.IgnorePaths, want.IgnorePaths)
		}
		if got.AllowedNetworks != nil || got.Users != nil || got.UsersFile != "" || got.SigningSecret != "" {
			t.Errorf("%q: authentication was configured from nothing: %+v", value, got)
		}
		// Options are read back without the driver behind them, so the
		// retention the environment did not configure is checked where it
		// is built.
		if got.Storage != nil {
			t.Errorf("%q: Options() returned a storage driver", value)
		}
		if size := envStorage(t).Cap(); size != want.RingBufferSize {
			t.Errorf("%q: storage holds %d traces, want the default %d", value, size, want.RingBufferSize)
		}
	}
}

func TestEnvListSetToSeparatorsKeepsTheDefault(t *testing.T) {
	// A list holding no entries is a list that was not set, so the defaults
	// stand rather than being wiped to nothing.
	t.Setenv("OIDA_IGNORE_PATHS", " , ,")
	t.Setenv("OIDA_ALLOWED_NETWORKS", ",,")

	tracer, err := New(NewOptions("env-test"))
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	got := tracer.Options()
	if want := NewOptions("env-test").WithDefaults(); !slices.Equal(got.IgnorePaths, want.IgnorePaths) {
		t.Errorf("ignore paths are %v, want the default %v", got.IgnorePaths, want.IgnorePaths)
	}
	if got.AllowedNetworks != nil {
		t.Errorf("allowed networks are %v, want none", got.AllowedNetworks)
	}
}
