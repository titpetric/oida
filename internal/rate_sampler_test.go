package internal

import "testing"

func TestRateSamplerIsDeterministic(t *testing.T) {
	sampler := NewRateSampler(50)

	sampled := 0
	for range 10 {
		if sampler.Sample(nil) {
			sampled++
		}
	}
	if sampled != 5 {
		t.Fatalf("sampled %d of 10 at a 50%% sample rate, want 5", sampled)
	}

	if NewRateSampler(0).Sample(nil) {
		t.Error("rate 0 sampled a request")
	}
	if !NewRateSampler(100).Sample(nil) {
		t.Error("rate 100 rejected a request")
	}
}
