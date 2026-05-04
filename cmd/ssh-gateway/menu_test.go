package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func sample() []VM {
	return []VM{
		{OpenstackID: "os-1", Name: "alpha", Status: "ACTIVE"},
		{OpenstackID: "os-2", Name: "beta", Status: "SHUTOFF"},
	}
}

func TestRenderMenu(t *testing.T) {
	var buf bytes.Buffer
	RenderMenu(&buf, sample())
	got := buf.String()
	if !strings.Contains(got, "1) alpha") || !strings.Contains(got, "(ACTIVE)") {
		t.Fatalf("missing alpha row: %q", got)
	}
	if !strings.Contains(got, "2) beta") || !strings.Contains(got, "(SHUTOFF)") {
		t.Fatalf("missing beta row: %q", got)
	}
	if !strings.Contains(got, "Select [1-2]:") {
		t.Fatalf("missing prompt: %q", got)
	}
}

func TestParseSelection_NumericOK(t *testing.T) {
	vm, err := ParseSelection("2", sample())
	if err != nil {
		t.Fatal(err)
	}
	if vm.Name != "beta" {
		t.Fatalf("got %q", vm.Name)
	}
}

func TestParseSelection_NumericTrim(t *testing.T) {
	vm, err := ParseSelection("  1  \n", sample())
	if err != nil || vm.Name != "alpha" {
		t.Fatalf("err=%v name=%q", err, vm.Name)
	}
}

func TestParseSelection_NumericOutOfRange(t *testing.T) {
	_, err := ParseSelection("9", sample())
	if !errors.Is(err, ErrSelectionInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestParseSelection_NameOK(t *testing.T) {
	vm, err := ParseSelection("beta", sample())
	if err != nil || vm.OpenstackID != "os-2" {
		t.Fatalf("err=%v vm=%+v", err, vm)
	}
}

func TestParseSelection_Empty(t *testing.T) {
	_, err := ParseSelection("", sample())
	if !errors.Is(err, ErrSelectionInvalid) {
		t.Fatalf("got %v", err)
	}
}

func TestFindByName(t *testing.T) {
	vm, ok := FindByName(sample(), "alpha")
	if !ok || vm.OpenstackID != "os-1" {
		t.Fatal("alpha not found")
	}
	if _, ok := FindByName(sample(), "missing"); ok {
		t.Fatal("missing should not be found")
	}
}
