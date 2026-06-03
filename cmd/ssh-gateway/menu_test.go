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
	want := "\r\nAvailable VMs:\r\n" +
		"  1) alpha                (ACTIVE)\r\n" +
		"  2) beta                 (SHUTOFF)\r\n" +
		"Select [1-2]: "
	if got != want {
		t.Fatalf("menu output:\ngot  %q\nwant %q", got, want)
	}
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

func TestApplyRuntimeInfoUsesOpenStackStatusAndAddress(t *testing.T) {
	vms := []VM{
		{OpenstackID: "os-1", Name: "alpha", Status: "BUILD"},
		{OpenstackID: "os-2", Name: "beta", Status: "BUILD"},
	}
	runtime := map[string]VMRuntime{
		"os-1": {Status: "ACTIVE", FixedIPv4: "10.0.0.7"},
		"os-2": {Status: "SHUTOFF", FixedIPv4: "10.0.0.8"},
	}

	got := applyRuntimeInfo(vms, func(vm VM) (VMRuntime, error) {
		return runtime[vm.OpenstackID], nil
	})

	if got[0].Status != "ACTIVE" || got[0].FixedIPv4 != "10.0.0.7" {
		t.Fatalf("first VM not refreshed: %+v", got[0])
	}
	if got[1].Status != "SHUTOFF" || got[1].FixedIPv4 != "10.0.0.8" {
		t.Fatalf("second VM not refreshed: %+v", got[1])
	}
}

func TestApplyRuntimeInfoMarksLookupFailuresUnknown(t *testing.T) {
	vms := []VM{{OpenstackID: "os-1", Name: "alpha", Status: "BUILD"}}

	got := applyRuntimeInfo(vms, func(vm VM) (VMRuntime, error) {
		return VMRuntime{}, errors.New("not found")
	})

	if got[0].Status != "UNKNOWN" {
		t.Fatalf("status = %q, want UNKNOWN", got[0].Status)
	}
	if got[0].FixedIPv4 != "" {
		t.Fatalf("fixed ip = %q, want empty", got[0].FixedIPv4)
	}
}
