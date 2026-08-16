package oida

import "testing"

func TestRateSamplerIsDeterministic(t *testing.T) {
	sampler := NewRateSampler(0.5)

	sampled := 0
	for range 10 {
		if sampler.Sample(nil) {
			sampled++
		}
	}
	if sampled != 5 {
		t.Fatalf("sampled %d of 10 at rate 0.5, want 5", sampled)
	}

	if NewRateSampler(0).Sample(nil) {
		t.Error("rate 0 sampled a request")
	}
	if !NewRateSampler(1).Sample(nil) {
		t.Error("rate 1 rejected a request")
	}
}
