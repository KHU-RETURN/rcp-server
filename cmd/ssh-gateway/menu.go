package main

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var ErrSelectionInvalid = errors.New("invalid selection")

func RenderMenu(w io.Writer, vms []VM) {
	_, _ = fmt.Fprintln(w, "Available VMs:")
	for i, vm := range vms {
		_, _ = fmt.Fprintf(w, "  %d) %-20s (%s)\r\n", i+1, vm.Name, vm.Status)
	}
	_, _ = fmt.Fprintf(w, "Select [1-%d]: ", len(vms))
}

// ParseSelection accepts either a 1-based index or an exact VM name.
func ParseSelection(input string, vms []VM) (VM, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return VM{}, ErrSelectionInvalid
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n < 1 || n > len(vms) {
			return VM{}, ErrSelectionInvalid
		}
		return vms[n-1], nil
	}
	if vm, ok := FindByName(vms, s); ok {
		return vm, nil
	}
	return VM{}, ErrSelectionInvalid
}

func FindByName(vms []VM, name string) (VM, bool) {
	for _, vm := range vms {
		if vm.Name == name {
			return vm, true
		}
	}
	return VM{}, false
}
