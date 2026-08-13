package container

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testClient points a Client at an httptest.Server via the same dialer
// seam NewDefaultClient uses for the real Unix socket/named pipe — the
// HTTP request/response handling is fully portable and testable this way
// without a real docker daemon.
func testClient(srv *httptest.Server) *Client {
	addr := srv.Listener.Addr().String()
	return newClientWithDialer(func(ctx context.Context) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	})
}

func TestListParsesContainersAndPorts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/json" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"Id":"abc123","Names":["/monkpayments-db-1"],"Image":"pgvector/pgvector:pg16","Ports":[{"PrivatePort":5432,"PublicPort":5434,"Type":"tcp"}]}]`))
	}))
	defer srv.Close()

	got, err := testClient(srv).List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d containers, want 1", len(got))
	}
	c := got[0]
	if c.ID != "abc123" || len(c.Names) != 1 || c.Names[0] != "/monkpayments-db-1" || c.Image != "pgvector/pgvector:pg16" {
		t.Fatalf("got %+v", c)
	}
	if len(c.Ports) != 1 || c.Ports[0] != (Port{PrivatePort: 5432, PublicPort: 5434, Type: "tcp"}) {
		t.Fatalf("got ports %+v", c.Ports)
	}
}

func TestListPropagatesNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := testClient(srv).List(context.Background())
	if err == nil {
		t.Fatalf("expected an error on a non-200 response")
	}
}

func TestStopSucceedsOn204And304(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusNotModified} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/containers/") || !strings.HasSuffix(r.URL.Path, "/stop") {
				t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			}
			w.WriteHeader(status)
		}))

		err := testClient(srv).Stop(context.Background(), "abc123")
		srv.Close()
		if err != nil {
			t.Fatalf("status %d: unexpected error: %v", status, err)
		}
	}
}

func TestStopReturnsErrorOnFailureStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if err := testClient(srv).Stop(context.Background(), "missing"); err == nil {
		t.Fatalf("expected an error for a 404 response")
	}
}
