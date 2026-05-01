package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// mustAllowlist는 CIDR spec을 파싱하고 에러 시 t.Fatal하는 테스트 헬퍼.
// package main 테스트 바이너리를 공유하므로 server_test.go/main_test.go에서 자유롭게 호출 가능.
func mustAllowlist(t *testing.T, cidr string) *Allowlist {
	t.Helper()
	al, err := ParseCIDRs(cidr)
	if err != nil {
		t.Fatalf("mustAllowlist(%q): %v", cidr, err)
	}
	return al
}

func TestNewLogger_LevelMapping(t *testing.T) {
	cases := []struct {
		in           string
		wantEnabled  slog.Level
		wantDisabled slog.Level // wantEnabled보다 한 단계 아래; "debug"일 때는 사용 안 함
		checkBelow   bool       // wantDisabled가 비활성인지 검사할지 여부
	}{
		{"debug", slog.LevelDebug, 0, false},             // debug 아래는 없음
		{"info", slog.LevelInfo, slog.LevelDebug, true},  // debug는 꺼져 있어야
		{"warn", slog.LevelWarn, slog.LevelInfo, true},   // info는 꺼져 있어야
		{"error", slog.LevelError, slog.LevelWarn, true}, // warn은 꺼져 있어야
		{"", slog.LevelInfo, slog.LevelDebug, true},      // default → info; debug는 꺼져 있어야
		{"weird", slog.LevelInfo, slog.LevelDebug, true}, // unknown → info; debug는 꺼져 있어야
	}
	for _, c := range cases {
		log := newLogger(c.in)
		if !log.Enabled(context.Background(), c.wantEnabled) {
			t.Errorf("newLogger(%q): want %v enabled", c.in, c.wantEnabled)
		}
		if c.checkBelow && log.Enabled(context.Background(), c.wantDisabled) {
			t.Errorf("newLogger(%q): %v should NOT be enabled", c.in, c.wantDisabled)
		}
	}
}

func TestStatsLoop_EmitsAtStartAndInterval(t *testing.T) {
	cfg := &Config{
		AllowList:   mustAllowlist(t, "127.0.0.0/8"),
		MaxConns:    4,
		DialTimeout: time.Second,
	}
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	srv := NewServer(cfg, log)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); statsLoop(ctx, srv, log, 50*time.Millisecond) }()

	// 최소 2회(시작 + 1 tick) 출력될 만큼 대기.
	time.Sleep(120 * time.Millisecond)
	cancel()
	<-done

	// "stats" 엔트리 개수 카운트.
	n := strings.Count(buf.String(), `msg=stats`)
	if n < 2 {
		t.Errorf("got %d stats lines, want >=2", n)
	}
}
