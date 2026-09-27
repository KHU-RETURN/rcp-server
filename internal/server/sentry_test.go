package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
)

type recordingTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (*recordingTransport) Configure(sentry.ClientOptions) {}
func (*recordingTransport) Flush(time.Duration) bool       { return true }
func (*recordingTransport) FlushWithContext(context.Context) bool {
	return true
}
func (*recordingTransport) Close() {}
func (t *recordingTransport) SendEvent(event *sentry.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, event)
}

func TestSentryMiddlewareCapturesServerErrorsAndPanics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	transport := &recordingTransport{}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:       "https://public@example.com/1",
		Transport: transport,
	})
	if err != nil {
		t.Fatal(err)
	}
	hub := sentry.NewHub(client, sentry.NewScope())

	router := gin.New()
	router.Use(gin.RecoveryWithWriter(io.Discard), sentrygin.New(sentrygin.Options{Repanic: true}), sentryServerErrorMiddleware())
	router.GET("/items/:id", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })
	router.GET("/panic", func(c *gin.Context) { panic("test panic") })
	router.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, path := range []string{"/items/secret-id?token=secret", "/panic", "/ok"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request = request.WithContext(sentry.SetHubOnContext(request.Context(), hub))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
	}
	client.Flush(2 * time.Second)

	transport.mu.Lock()
	defer transport.mu.Unlock()
	if len(transport.events) != 2 {
		t.Fatalf("captured %d events, want 2", len(transport.events))
	}
	var sawStatus, sawPanic bool
	for _, event := range transport.events {
		if event.Message == "GET /items/:id returned 500" {
			sawStatus = true
			if event.Level != sentry.LevelError {
				t.Fatalf("5xx event level = %q, want error", event.Level)
			}
		}
		if len(event.Exception) > 0 || strings.Contains(event.Message, "test panic") {
			sawPanic = true
		}
		if strings.Contains(event.Message, "secret") {
			t.Fatalf("event message contains request parameter: %q", event.Message)
		}
	}
	if !sawStatus || !sawPanic {
		t.Fatalf("missing status or panic event: status=%t panic=%t", sawStatus, sawPanic)
	}
}
