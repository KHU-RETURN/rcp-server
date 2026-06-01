package main

import (
	"sync"
	"testing"
	"time"
)

func TestWaitForCopyDoneTimesOut(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	if waitForCopyDone(&wg, 10*time.Millisecond) {
		t.Fatal("expected blocked copy wait to time out")
	}
}

func TestWaitForCopyDoneReturnsWhenDone(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	go wg.Done()
	if !waitForCopyDone(&wg, time.Second) {
		t.Fatal("expected completed copy wait")
	}
}

func TestWaitForInnerOrOuterClosesInnerOnOuterDone(t *testing.T) {
	innerWait := make(chan error)
	outerDone := make(chan struct{})
	closed := make(chan struct{})

	close(outerDone)
	err := waitForInnerOrOuter(innerWait, outerDone, func() { close(closed) }, 10*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout waiting for inner exit")
	}
	select {
	case <-closed:
	default:
		t.Fatal("expected closeInner to be called")
	}
}

func TestWaitForInnerOrOuterReturnsInnerExit(t *testing.T) {
	innerWait := make(chan error, 1)
	outerDone := make(chan struct{})
	innerWait <- nil
	if err := waitForInnerOrOuter(innerWait, outerDone, func() { t.Fatal("unexpected close") }, time.Second); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}
