package frontend

import "testing"

func TestLoginBackTarget(t *testing.T) {
	const base = "/debug/oida"

	// A location inside the dashboard is followed, query included.
	for _, back := range []string{
		base + "/traces",
		base + "/traces?kind=db",
		base + "/trace/abc123",
		base + "/index.php?foo=bar",
	} {
		if got := backTarget(base, back); got != back {
			t.Errorf("backTarget(%q) = %q, want it followed", back, got)
		}
	}

	// Anything else lands on the overview: other sites, protocol relative
	// addresses, backslash tricks, foreign paths, the bare base, nothing.
	for _, back := range []string{
		"https://evil.example/phish",
		"//evil.example/phish",
		base + "//evil.example",
		base + `/\evil.example`,
		"/etc/passwd",
		base,
		"",
	} {
		if got := backTarget(base, back); got != base {
			t.Errorf("backTarget(%q) = %q, want the overview", back, got)
		}
	}
}
