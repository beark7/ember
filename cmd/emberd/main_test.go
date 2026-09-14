package main

import "testing"

func TestRunVersion(t *testing.T) {
	if code := run([]string{"--version"}); code != 0 {
		t.Fatalf("--version exit code = %d, want 0", code)
	}
}

func TestRunWithoutArgsIsNotImplemented(t *testing.T) {
	if code := run(nil); code != 1 {
		t.Fatalf("no-args exit code = %d, want 1", code)
	}
}

func TestRunBadFlag(t *testing.T) {
	if code := run([]string{"--no-such-flag"}); code != 2 {
		t.Fatalf("bad flag exit code = %d, want 2", code)
	}
}
