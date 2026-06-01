package main

import (
	"bytes"
	"io"
	"testing"
)

type scriptedReadWriter struct {
	in  *bytes.Reader
	out bytes.Buffer
}

func newScriptedReadWriter(input string) *scriptedReadWriter {
	return &scriptedReadWriter{in: bytes.NewReader([]byte(input))}
}

func (rw *scriptedReadWriter) Read(p []byte) (int, error) {
	return rw.in.Read(p)
}

func (rw *scriptedReadWriter) Write(p []byte) (int, error) {
	return rw.out.Write(p)
}

func TestReadLineEchoesInputToTerminal(t *testing.T) {
	rw := newScriptedReadWriter("2\r")

	got, err := readLine(rw, make([]byte, 64))
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "2" {
		t.Fatalf("got %q", got)
	}
	if rw.out.String() != "2\r\n" {
		t.Fatalf("echo got %q", rw.out.String())
	}
}

func TestReadLineEchoesBackspace(t *testing.T) {
	rw := newScriptedReadWriter("12\x7f3\n")

	got, err := readLine(rw, make([]byte, 64))
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "13" {
		t.Fatalf("got %q", got)
	}
	if rw.out.String() != "12\b \b3\r\n" {
		t.Fatalf("echo got %q", rw.out.String())
	}
}
