package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestSetupSendsToBraintrust(t *testing.T) {
	var mu sync.Mutex
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = r
		mu.Unlock()
	}))
	defer srv.Close()
	t.Cleanup(func() { Use(noopProvider(), false) })

	flush, err := Setup(context.Background(), Config{APIKey: "sk-test", APIURL: srv.URL + "/", Project: "pidex", Service: "pidex test"})
	if err != nil {
		t.Fatal(err)
	}
	_, sp := Start(context.Background(), "chat turn", Task)
	sp.End(nil)
	if err := flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if got == nil {
		t.Fatal("nothing was sent")
	}
	if got.URL.Path != "/otel/v1/traces" {
		t.Errorf("sent to %s", got.URL.Path)
	}
	if got.Header.Get("Authorization") != "Bearer sk-test" || got.Header.Get("x-bt-parent") != "project_name:pidex" {
		t.Errorf("headers = %v", got.Header)
	}
}

func TestNoKeyNoTraces(t *testing.T) {
	Use(noopProvider(), false)
	flush, err := Setup(context.Background(), Config{Project: "pidex"})
	if err != nil {
		t.Fatal(err)
	}
	defer flush(context.Background())
	ctx, sp := Start(context.Background(), "turn", Task)
	if sp != nil || From(ctx) != nil {
		t.Fatal("a span was started with tracing off")
	}
	// A nil span takes every call.
	sp.Input("x")
	sp.Set("k", 1)
	sp.Metric("tokens", 1)
	sp.End(nil)
}
