package kube

import (
	"testing"
	"time"
)

func TestPodLogOptionsFollow(test *testing.T) {
	opts := PodLogOptions("app", 200, true)
	if opts.Container != "app" {
		test.Fatalf("container = %q", opts.Container)
	}
	if opts.TailLines == nil || *opts.TailLines != 200 {
		test.Fatalf("tail = %v", opts.TailLines)
	}
	if !opts.Follow {
		test.Fatal("expected Follow=true")
	}

	oneShot := PodLogOptions("app", 0, false)
	if oneShot.Follow {
		test.Fatal("expected Follow=false")
	}
	if oneShot.TailLines == nil || *oneShot.TailLines != defaultTail {
		test.Fatalf("default tail = %v", oneShot.TailLines)
	}
}

func TestDefaultTimeoutStillUsedForUnaryCalls(test *testing.T) {
	// Guardrails: unary ops keep their own deadlines; follow must not inherit a short HTTP Timeout.
	if defaultTimeout < time.Second {
		test.Fatalf("defaultTimeout = %s", defaultTimeout)
	}
	if logsTimeout < defaultTimeout {
		test.Fatalf("logsTimeout = %s should be >= defaultTimeout", logsTimeout)
	}
}
