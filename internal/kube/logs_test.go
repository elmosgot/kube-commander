package kube

import (
	"testing"
	"time"
)

func TestPodLogOptionsFollow(t *testing.T) {
	opts := PodLogOptions("app", 200, true)
	if opts.Container != "app" {
		t.Fatalf("container = %q", opts.Container)
	}
	if opts.TailLines == nil || *opts.TailLines != 200 {
		t.Fatalf("tail = %v", opts.TailLines)
	}
	if !opts.Follow {
		t.Fatal("expected Follow=true")
	}

	oneShot := PodLogOptions("app", 0, false)
	if oneShot.Follow {
		t.Fatal("expected Follow=false")
	}
	if oneShot.TailLines == nil || *oneShot.TailLines != defaultTail {
		t.Fatalf("default tail = %v", oneShot.TailLines)
	}
}

func TestDefaultTimeoutStillUsedForUnaryCalls(t *testing.T) {
	// Guardrails: unary ops keep their own deadlines; follow must not inherit a short HTTP Timeout.
	if defaultTimeout < time.Second {
		t.Fatalf("defaultTimeout = %s", defaultTimeout)
	}
	if logsTimeout < defaultTimeout {
		t.Fatalf("logsTimeout = %s should be >= defaultTimeout", logsTimeout)
	}
}
