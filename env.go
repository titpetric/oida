package oida

import (
	"math"
	"os"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// optionsFromEnv applies the environment to opts. A variable applies only
// where the code left the field at its NewOptions default, so authentication
// and tuning configured in code win over the environment.
//
// The variables, with the value the option keeps when one is unset:
//
//	OIDA_SERVICE_NAME        the name passed to NewOptions
//	OIDA_PATH                /debug/oida
//	OIDA_ENABLED             false: recording stays off
//	OIDA_RING_BUFFER_SIZE    200
//	OIDA_TOP_REQUESTS        20
//	OIDA_MAX_SPANS_PER_TRACE 1000
//	OIDA_SAMPLE_RATE         100
//	OIDA_TRACK_MEMORY_USE    true
//	OIDA_TRUST_REQUEST_ID    false
//	OIDA_REFRESH_INTERVAL    5
//	OIDA_LIVE_STREAM         true
//	OIDA_IGNORE_PATHS        /healthz,/readyz,/metrics,/favicon.ico
//	OIDA_ALLOWED_NETWORKS    none (every peer is served)
//	OIDA_AUTH                none (no sign in screen)
//	OIDA_USERS_FILE          none
//	OIDA_SIGNING_SECRET      none (a per-process secret is generated)
//
// Lists are comma separated. OIDA_SAMPLE_RATE out of [0,100] clamps to the
// nearest bound. OIDA_AUTH holds one username:password pair, hashed here so
// the options carry a bcrypt hash the way a configured deployment would.
func optionsFromEnv(opts *Options) error {
	defaults := NewOptions("")

	if opts.ServiceName == "" {
		opts.ServiceName = os.Getenv("OIDA_SERVICE_NAME")
	}
	if value := os.Getenv("OIDA_PATH"); value != "" && opts.Path == defaults.Path {
		opts.Path = value
	}
	if value := os.Getenv("OIDA_USERS_FILE"); value != "" && opts.UsersFile == "" {
		opts.UsersFile = value
	}
	if opts.SigningSecret == "" {
		opts.SigningSecret = os.Getenv("OIDA_SIGNING_SECRET")
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

	if value := os.Getenv("OIDA_SAMPLE_RATE"); value != "" && opts.SampleRate == defaults.SampleRate {
		rate, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(rate) {
			return invalidOption("OIDA_SAMPLE_RATE", "must be a number")
		}
		// Out of bounds clamps to the valid range instead of failing startup.
		opts.SampleRate = min(max(rate, 0), 100)
	}

	if value := os.Getenv("OIDA_IGNORE_PATHS"); value != "" && slices.Equal(opts.IgnorePaths, defaults.IgnorePaths) {
		opts.IgnorePaths = splitList(value)
	}
	if value := os.Getenv("OIDA_ALLOWED_NETWORKS"); value != "" && opts.AllowedNetworks == nil {
		opts.AllowedNetworks = splitList(value)
	}

	return authFromEnv(opts)
}

// authFromEnv applies the environment's authentication opt-in: OIDA_AUTH is
// one username:password pair. Users configured in code win.
func authFromEnv(opts *Options) error {
	credentials := os.Getenv("OIDA_AUTH")
	if credentials == "" || opts.Users != nil || opts.UsersFile != "" || opts.AuthorizeUser != nil {
		return nil
	}
	username, password, ok := strings.Cut(credentials, ":")
	if !ok || username == "" || password == "" {
		return invalidOption("OIDA_AUTH", "must be username:password")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	opts.Users = map[string]string{username: string(hash)}
	return nil
}

// envBool applies a boolean variable when the field still holds its default.
func envBool(name string, field *bool, unset bool) error {
	value := os.Getenv(name)
	if value == "" || *field != unset {
		return nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return invalidOption(name, "must be true or false")
	}
	*field = parsed
	return nil
}

// envInt applies an integer variable when the field still holds its default.
func envInt(name string, field *int, unset int) error {
	value := os.Getenv(name)
	if value == "" || *field != unset {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return invalidOption(name, "must be an integer")
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
