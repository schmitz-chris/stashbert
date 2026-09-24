package main

import (
	"bytes"
	"testing"
)

// noEnv is a getenv without any variables.
func noEnv(string) string { return "" }

func TestParseFlagsVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	done, err := parseFlags(t.Context(), []string{"-version"}, "v1.2.3-4-gabcdef0", noEnv, &stdout, &stderr)
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if !done {
		t.Error("done = false, want true (the server must not start)")
	}
	if got, want := stdout.String(), "v1.2.3-4-gabcdef0\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestParseFlagsNone(t *testing.T) {
	var stdout, stderr bytes.Buffer
	done, err := parseFlags(t.Context(), nil, "dev", noEnv, &stdout, &stderr)
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if done {
		t.Error("done = true, want false (the server must start)")
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("stdout = %q, stderr = %q, want both empty", stdout.String(), stderr.String())
	}
}

func TestParseFlagsUnknown(t *testing.T) {
	var stdout, stderr bytes.Buffer
	done, err := parseFlags(t.Context(), []string{"-nope"}, "dev", noEnv, &stdout, &stderr)
	if err == nil {
		t.Fatal("parseFlags: err = nil, want an error")
	}
	if done {
		t.Error("done = true, want false")
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		args []string
		want int
	}{
		{[]string{"-h"}, 0},
		{[]string{"-nope"}, 2},
	}
	for _, tt := range tests {
		var stdout, stderr bytes.Buffer
		_, err := parseFlags(t.Context(), tt.args, "dev", noEnv, &stdout, &stderr)
		if err == nil {
			t.Fatalf("parseFlags(%q): err = nil, want an error", tt.args)
		}
		if got := exitCode(err); got != tt.want {
			t.Errorf("exitCode for %q = %d, want %d", tt.args, got, tt.want)
		}
	}
}
