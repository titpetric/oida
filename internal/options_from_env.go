package internal

import (
	"context"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/titpetric/oida/model"
	"github.com/titpetric/oida/storage"
)

// OptionsFromEnv applies the OIDA_* environment to opts. A variable applies
// only where the code left the field at its NewOptions default, so
// authentication and tuning configured in code win over the environment, and a
// variable set to nothing leaves the default alone.
//
// The variables and their defaults are the table in docs/guide-configuration.md,
// which is where a deployment reads them; keeping the list in one place is what
// keeps it true. Lists are comma separated. OIDA_SAMPLE_RATE out of [0,100]
// clamps to the nearest bound. OIDA_AUTH holds one username:password pair,
// hashed here so the options carry a bcrypt hash the way a configured
// deployment would.
func OptionsFromEnv(opts *model.Options) error {
	defaults := model.NewOptions("")

	if opts.ServiceName == "" {
		opts.ServiceName = envValue("OIDA_SERVICE_NAME")
	}
	if value := envValue("OIDA_PATH"); value != "" && opts.Path == defaults.Path {
		opts.Path = value
	}
	if value := envValue("OIDA_USERS_FILE"); value != "" && opts.UsersFile == "" {
		opts.UsersFile = value
	}
	if opts.SigningSecret == "" {
		opts.SigningSecret = envValue("OIDA_SIGNING_SECRET")
	}

	if err := envBool("OIDA_ENABLED", &opts.Enabled, defaults.Enabled); err != nil {
		return err
	}
	if err := envBool("OIDA_TRACK_MEMORY_USE", &opts.TrackMemoryUse, defaults.TrackMemoryUse); err != nil {
		return err
	}
	if err := envBool("OIDA_TRUST_REQUEST_ID", &opts.TrustRequestID, defaults.TrustRequestID); err != nil {
		return err
	}
	if err := envBool("OIDA_LIVE_STREAM", &opts.LiveStream, defaults.LiveStream); err != nil {
		return err
	}
	if err := envBool("OIDA_CAPTURE_LOGS", &opts.CaptureLogs, defaults.CaptureLogs); err != nil {
		return err
	}

	if err := envInt("OIDA_RING_BUFFER_SIZE", &opts.RingBufferSize, defaults.RingBufferSize); err != nil {
		return err
	}
	if err := envInt("OIDA_TOP_REQUESTS", &opts.TopRequests, defaults.TopRequests); err != nil {
		return err
	}
	if err := envInt("OIDA_MAX_SPANS_PER_TRACE", &opts.MaxSpansPerTrace, defaults.MaxSpansPerTrace); err != nil {
		return err
	}
	if err := envInt("OIDA_REFRESH_INTERVAL", &opts.RefreshInterval, defaults.RefreshInterval); err != nil {
		return err
	}

	if value := envValue("OIDA_SAMPLE_RATE"); value != "" && opts.SampleRate == defaults.SampleRate {
		rate, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(rate) {
			return InvalidOption("OIDA_SAMPLE_RATE", "must be a number")
		}
		// Out of bounds clamps to the valid range instead of failing startup.
		opts.SampleRate = min(max(rate, 0), 100)
	}

	if list := splitList(envValue("OIDA_IGNORE_PATHS")); len(list) > 0 && slices.Equal(opts.IgnorePaths, defaults.IgnorePaths) {
		opts.IgnorePaths = list
	}
	if list := splitList(envValue("OIDA_ALLOWED_NETWORKS")); len(list) > 0 && opts.AllowedNetworks == nil {
		opts.AllowedNetworks = list
	}

	if err := storageFromEnv(opts); err != nil {
		return err
	}

	return authFromEnv(opts)
}

// The storage drivers OIDA_STORAGE_DRIVER selects.
const (
	driverMemory = "memory"
	driverDisk   = "disk"
)

// storageFromEnv builds the retention driver the environment asks for.
// Storage set in code wins, so the variables are read only when the field is
// nil.
//
// A driver takes its settings from the variables carrying its name, and the
// two are sized by RingBufferSize until one says otherwise. The driver
// defaults to memory, and to disk when a disk setting is given on its own,
// which is the only thing such a setting can mean. Naming one driver and
// configuring the other is a contradiction and fails, as does a path that
// cannot be created or written.
func storageFromEnv(opts *model.Options) error {
	if opts.Storage != nil {
		return nil
	}
	driver := envValue("OIDA_STORAGE_DRIVER")
	path := envValue("OIDA_STORAGE_DISK_PATH")
	diskSet := path != "" ||
		envValue("OIDA_STORAGE_DISK_LIMIT") != "" ||
		envValue("OIDA_STORAGE_DISK_LIST") != "" ||
		envValue("OIDA_STORAGE_DISK_EXPIRE") != ""

	switch driver {
	case "":
		if !diskSet {
			return storageMemoryFromEnv(opts)
		}
	case driverMemory:
		if diskSet {
			return InvalidOption("OIDA_STORAGE_DRIVER", "the memory driver has no OIDA_STORAGE_DISK_ settings")
		}
		return storageMemoryFromEnv(opts)
	case driverDisk:
	default:
		return InvalidOption("OIDA_STORAGE_DRIVER", "must be "+driverMemory+" or "+driverDisk)
	}

	limit := opts.RingBufferSize
	if err := envInt("OIDA_STORAGE_DISK_LIMIT", &limit, limit); err != nil {
		return err
	}
	list := false
	if err := envBool("OIDA_STORAGE_DISK_LIST", &list, false); err != nil {
		return err
	}
	expire, err := envDuration("OIDA_STORAGE_DISK_EXPIRE")
	if err != nil {
		return err
	}
	var paths []string
	if path != "" {
		paths = append(paths, path)
	}
	store, err := storage.NewDiskStorage(limit, paths...)
	if err != nil {
		return err
	}

	ctx := context.Background()
	// Ageing the folder out comes first, so nothing about to be dropped is
	// read back. This is one pass, at startup: a process that wants a folder
	// aged out continuously puts Prune on a ticker of its own.
	if expire > 0 {
		if err := store.Prune(ctx, expire); err != nil {
			return err
		}
	}
	// Reading the folder back is off by default: it costs a listing and a
	// decode per document at startup, and a dashboard that lists what this
	// process recorded is what most deployments want.
	if list {
		if err := store.Restore(ctx); err != nil {
			return err
		}
	}
	opts.Storage = store
	return nil
}

// storageMemoryFromEnv builds the memory driver, which is what an unset
// driver resolves to. It is left nil at its default size, so the tracer builds
// it from RingBufferSize the way it does for options that never saw the
// environment.
func storageMemoryFromEnv(opts *model.Options) error {
	size := opts.RingBufferSize
	if err := envInt("OIDA_STORAGE_MEMORY_SIZE", &size, size); err != nil {
		return err
	}
	if size == opts.RingBufferSize {
		return nil
	}
	opts.Storage = storage.NewMemoryStorage(size)
	return nil
}

// authFromEnv applies the environment's authentication opt-in: OIDA_AUTH is
// one username:password pair. Users configured in code win.
func authFromEnv(opts *model.Options) error {
	credentials := envValue("OIDA_AUTH")
	if credentials == "" || opts.Users != nil || opts.UsersFile != "" || opts.AuthorizeUser != nil {
		return nil
	}
	username, password, ok := strings.Cut(credentials, ":")
	if !ok || username == "" || password == "" {
		return InvalidOption("OIDA_AUTH", "must be username:password")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	opts.Users = map[string]string{username: string(hash)}
	return nil
}

// envValue reads one variable, with a value of nothing but whitespace read as
// an unset one. Every read goes through here, so a variable set to the empty
// string leaves the option at its default rather than at the zero value.
func envValue(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

// envBool applies a boolean variable when the field still holds its default.
func envBool(name string, field *bool, unset bool) error {
	value := envValue(name)
	if value == "" || *field != unset {
		return nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return InvalidOption(name, "must be true or false")
	}
	*field = parsed
	return nil
}

// envDuration reads a duration variable, such as 168h. It is unset as zero,
// and a value that is not a positive duration fails rather than being read as
// one.
func envDuration(name string) (time.Duration, error) {
	value := envValue(name)
	if value == "" {
		return 0, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, InvalidOption(name, "must be a duration, such as 168h")
	}
	if parsed <= 0 {
		return 0, InvalidOption(name, "must be a positive duration")
	}
	return parsed, nil
}

// envInt applies an integer variable when the field still holds its default.
func envInt(name string, field *int, unset int) error {
	value := envValue(name)
	if value == "" || *field != unset {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return InvalidOption(name, "must be an integer")
	}
	*field = parsed
	return nil
}

// splitList splits a comma separated variable, trimming blanks.
func splitList(value string) []string {
	var list []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			list = append(list, item)
		}
	}
	return list
}
