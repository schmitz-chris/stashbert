package main

import (
	"bytes"
	"testing"
)

func TestParseFlagsVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	done, err := parseFlags([]string{"-version"}, "v1.2.3-4-gabcdef0", &stdout, &stderr)
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
	done, err := parseFlags(nil, "dev", &stdout, &stderr)
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
	done, err := parseFlags([]string{"-nope"}, "dev", &stdout, &stderr)
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
