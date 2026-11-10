package nodeporthandler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	v1 "k8s.io/api/core/v1"
)

var tcpPort = v1.ServicePort{Protocol: v1.ProtocolTCP, NodePort: 30080}

func TestExpose_SendsTokenAndBody(t *testing.T) {
	t.Setenv("CRC_REST_API_TOKEN", "test-token")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBearerToken(t, r, "test-token")
		assertBody(t, r, map[string]string{"Local": "127.0.0.1:30080", "Remote": "192.168.127.2:30080"})
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	overrideURL(t, &exposeURL, srv.URL)

	if err := expose(tcpPort); err != nil {
		t.Fatalf("expose returned unexpected error: %v", err)
	}
}

func TestUnexpose_SendsTokenAndBody(t *testing.T) {
	t.Setenv("CRC_REST_API_TOKEN", "test-token")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBearerToken(t, r, "test-token")
		assertBody(t, r, map[string]string{"Local": "127.0.0.1:30080"})
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	overrideURL(t, &unexposeURL, srv.URL)

	if err := unexpose(tcpPort); err != nil {
		t.Fatalf("unexpose returned unexpected error: %v", err)
	}
}

func TestExposeUnexpose_Non2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	overrideURL(t, &exposeURL, srv.URL)
	overrideURL(t, &unexposeURL, srv.URL)

	if err := expose(tcpPort); err == nil {
		t.Error("expected expose error on 401, got nil")
	}
	if err := unexpose(tcpPort); err == nil {
		t.Error("expected unexpose error on 401, got nil")
	}
}

func TestExposeUnexpose_SkipsNonTCP(t *testing.T) {
	// atomic.Int32 because httptest.Server handles each request in its own goroutine.
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	overrideURL(t, &exposeURL, srv.URL)
	overrideURL(t, &unexposeURL, srv.URL)

	for _, protocol := range []v1.Protocol{v1.ProtocolUDP, v1.ProtocolSCTP} {
		port := v1.ServicePort{Protocol: protocol, NodePort: 30053}
		if err := expose(port); err != nil {
			t.Errorf("expose %s returned unexpected error: %v", protocol, err)
		}
		if err := unexpose(port); err != nil {
			t.Errorf("unexpose %s returned unexpected error: %v", protocol, err)
		}
	}
	if calls.Load() != 0 {
		t.Errorf("expected no requests for non-TCP ports, got %d", calls.Load())
	}
}

func TestForwarderURLs(t *testing.T) {
	if exposeURL != "http://host/services/forwarder/expose" {
		t.Errorf("unexpected exposeURL %q", exposeURL)
	}
	if unexposeURL != "http://host/services/forwarder/unexpose" {
		t.Errorf("unexpected unexposeURL %q", unexposeURL)
	}
}

func overrideURL(t *testing.T, target *string, url string) {
	t.Helper()
	orig := *target
	*target = url
	t.Cleanup(func() { *target = orig })
}

func assertBearerToken(t *testing.T, r *http.Request, token string) {
	t.Helper()
	if auth := r.Header.Get("Authorization"); auth != "Bearer "+token {
		t.Errorf("expected Authorization %q, got %q", "Bearer "+token, auth)
	}
}

func assertBody(t *testing.T, r *http.Request, expected map[string]string) {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("failed to read request body: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("failed to unmarshal body: %v", err)
	}
	if len(got) != len(expected) {
		t.Errorf("expected body %v, got %v", expected, got)
	}
	for k, v := range expected {
		if got[k] != v {
			t.Errorf("expected %s=%q, got %q", k, v, got[k])
		}
	}
}
