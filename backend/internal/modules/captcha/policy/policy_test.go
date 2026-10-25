package policy

import (
	"testing"
	"time"
)

func TestFailurePolicyDefaults(t *testing.T) {
	failure := Failure()
	if failure.Limit != 15 {
		t.Fatalf("limit = %d, want 15", failure.Limit)
	}
	if failure.Window != 10*time.Minute {
		t.Fatalf("window = %s, want 10m", failure.Window)
	}
	if failure.Cooldown != 10*time.Minute {
		t.Fatalf("cooldown = %s, want 10m", failure.Cooldown)
	}
}

func TestFailureKeyIsSharedByIP(t *testing.T) {
	if got := FailureKey("203.0.113.9"); got != "captcha-fail:ip:203.0.113.9" {
		t.Fatalf("key = %q, want captcha-fail:ip:203.0.113.9", got)
	}
}
