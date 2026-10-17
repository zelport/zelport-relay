package relay

import (
	"bytes"
	"context"
	"github.com/zelport/zelport-sdk-go/protocol"
	"github.com/zelport/zelport-sdk-go/transport"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRelayServesOnlyV1Endpoint(t *testing.T) {
	s, err := New(Options{AllowUnauthenticated: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	for _, path := range []string{"/v0/ws", "/v2/ws", "/v99/ws"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			response := httptest.NewRecorder()
			s.ServeHTTP(response, request)
			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
			}
		})
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/ws", nil)
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code == http.StatusNotFound {
		t.Fatal("current relay endpoint returned 404")
	}
}

func TestRelayRequiresAdmission(t *testing.T) {
	s, e := New(Options{AccessToken: bytes.Repeat([]byte{1}, 32)})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	server := httptest.NewServer(s)
	defer server.Close()
	if c, e := transport.Dial(context.Background(), "ws"+server.URL[4:], bytes.Repeat([]byte{2}, 32), "control"); e == nil {
		c.Close()
		t.Fatal("wrong token admitted")
	}
}
func TestRelayDuplicateRegistration(t *testing.T) {
	token := bytes.Repeat([]byte{1}, 32)
	s, _ := New(Options{AccessToken: token})
	defer s.Close()
	server := httptest.NewServer(s)
	defer server.Close()
	raw, _ := protocol.EncodeCatalogEnvelope(protocol.CatalogEnvelope{Version: 1, Group: string(bytes.Repeat([]byte("a"), 64)), RouteID: string(bytes.Repeat([]byte("b"), 64)), Nonce: make([]byte, 24), Ciphertext: make([]byte, 16)})
	for i := 0; i < 2; i++ {
		c, e := transport.Dial(context.Background(), "ws"+server.URL[4:], token, "control")
		if e != nil {
			t.Fatal(e)
		}
		defer c.Close()
		if e = protocol.WriteJSON(c, protocol.Message{Type: "register", ID: protocol.ID(), Record: raw}); e != nil {
			t.Fatal(e)
		}
		var reply protocol.Message
		for {
			if e = protocol.ReadJSON(c, &reply); e != nil {
				t.Fatal(e)
			}
			if reply.Type == "result" {
				break
			}
		}
		if i == 0 && reply.Error != "" {
			t.Fatal(reply.Error)
		}
		if i == 1 && reply.Error != "duplicate_identity" {
			t.Fatal(reply)
		}
	}
}

func TestPairedTunnelReleasesPendingCapacityAndStopsWithControl(t *testing.T) {
	s, _ := New(Options{AccessToken: bytes.Repeat([]byte{1}, 32)})
	a, ac := net.Pipe()
	b, bc := net.Pipe()
	defer a.Close()
	defer ac.Close()
	defer b.Close()
	defer bc.Close()
	consumer := &control{cap: protocol.RandomNonce(), done: make(chan struct{}), out: make(chan protocol.Message, 4)}
	provider := &control{cap: protocol.RandomNonce(), done: make(chan struct{}), out: make(chan protocol.Message, 4)}
	peer := protocol.RelayRoute{Version: protocol.Version, Group: string(bytes.Repeat([]byte("a"), 64)), ConsumerRoute: string(bytes.Repeat([]byte("b"), 64)), ProviderRoute: string(bytes.Repeat([]byte("c"), 64)), Session: protocol.RandomNonce()}
	s.devices[peer.Group+"/"+peer.ConsumerRoute] = consumer
	s.devices[peer.Group+"/"+peer.ProviderRoute] = provider
	ac.SetDeadline(time.Now().Add(2 * time.Second))
	bc.SetDeadline(time.Now().Add(2 * time.Second))
	done := make(chan struct{})
	go func() { s.connect(a); close(done) }()
	if e := protocol.WriteJSON(ac, protocol.Message{Type: "connect", Route: &peer, Control: consumer.cap}); e != nil {
		t.Fatal(e)
	}
	offer := <-provider.out
	joined := make(chan struct{})
	go func() { s.join(b); close(joined) }()
	if e := protocol.WriteJSON(bc, protocol.Message{Type: "join", Route: &peer, Control: provider.cap, Capability: offer.Capability}); e != nil {
		t.Fatal(e)
	}
	var reply protocol.Message
	if e := protocol.ReadJSON(bc, &reply); e != nil {
		t.Fatal(e)
	}
	if e := protocol.ReadJSON(ac, &reply); e != nil {
		t.Fatal(e)
	}
	s.mu.Lock()
	pending := s.pendingCount
	s.mu.Unlock()
	if pending != 0 {
		t.Fatal("active tunnel counted as pending", pending)
	}
	close(provider.done)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("control loss left tunnel active")
	}
	select {
	case <-joined:
	case <-time.After(time.Second):
		t.Fatal("join owner did not finish")
	}
}
