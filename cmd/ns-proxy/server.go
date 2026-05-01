package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	socks5 "github.com/things-go/go-socks5"
)

const shutdownPollInterval = 50 * time.Millisecond

// Server는 *socks5.Server를 동시 접속 semaphore + graceful shutdown으로 감싼다.
// listener는 호출자 소유: close → accept loop 종료, Shutdown으로 drain.
type Server struct {
	log         *slog.Logger
	socksServer *socks5.Server
	sem         chan struct{}
	wg          sync.WaitGroup

	serveDone chan struct{}
	serveOnce sync.Once

	activeConns int64 // sync/atomic으로만 접근
	totalDials  int64
	deniedDials int64
}

func NewServer(cfg *Config, log *slog.Logger) *Server {
	return &Server{
		log:         log,
		socksServer: NewSOCKS5Server(cfg, log),
		sem:         make(chan struct{}, cfg.MaxConns),
		serveDone:   make(chan struct{}),
	}
}

// Serve는 ln의 accept loop를 돈다. ctx는 API 대칭으로 받지만 무시 — 종료는
// 호출자가 ln을 close. transient accept 에러는 backoff-retry로 흡수.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	defer s.serveOnce.Do(func() { close(s.serveDone) })

	var tempDelay time.Duration
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				s.wg.Wait()
				return nil
			}
			if tempDelay == 0 {
				tempDelay = 10 * time.Millisecond
			} else {
				tempDelay *= 2
			}
			if tempDelay > time.Second {
				tempDelay = time.Second
			}
			s.log.Warn("accept transient error, retrying", "err", err, "delay", tempDelay)
			time.Sleep(tempDelay)
			continue
		}
		tempDelay = 0

		select {
		case s.sem <- struct{}{}:
			atomic.AddInt64(&s.totalDials, 1)
			atomic.AddInt64(&s.activeConns, 1)
			s.wg.Add(1)
			go func() {
				// defer LIFO: sem 해제 먼저 (슬롯 즉시 회수) → activeConns-- → wg.Done 마지막
				// (Shutdown의 wg.Wait이 모든 goroutine 종료를 보게).
				defer s.wg.Done()
				defer atomic.AddInt64(&s.activeConns, -1)
				defer func() { <-s.sem }()
				// TODO(P2): lib에 handshake-only deadline 훅이 없어, slow-handshake
				// 클라이언트가 슬롯을 무한 점유 가능. 별도 이슈로 분리.
				if err := s.socksServer.ServeConn(conn); err != nil {
					if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
						s.log.Debug("serve conn closed", "err", err)
					} else {
						s.log.Info("serve conn closed", "err", err)
					}
				}
			}()
		default:
			// 슬롯 만석 → SOCKS5 협상 전에 TCP를 끊는 것이 가장 명확한 신호.
			atomic.AddInt64(&s.deniedDials, 1)
			s.log.Warn("rejected: max conns reached")
			_ = conn.Close()
		}
	}
}

// Shutdown: (1) accept loop 종료 대기 → (2) in-flight 핸들러 drain.
// 호출자는 먼저 listener를 close해야 함. drain 완료 시 nil, ctx 만료 시 ctx.Err().
func (s *Server) Shutdown(ctx context.Context) error {
	select {
	case <-s.serveDone:
	case <-ctx.Done():
		return ctx.Err()
	}
	ticker := time.NewTicker(shutdownPollInterval)
	defer ticker.Stop()
	for {
		if atomic.LoadInt64(&s.activeConns) == 0 {
			s.wg.Wait()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *Server) Stats() (active, total, denied int64) {
	return atomic.LoadInt64(&s.activeConns),
		atomic.LoadInt64(&s.totalDials),
		atomic.LoadInt64(&s.deniedDials)
}
