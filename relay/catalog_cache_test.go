package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/zelport/zelport-sdk-go/protocol"
	"github.com/zelport/zelport-sdk-go/transport"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCatalogKnownCiphertext(t *testing.T) {
	token := bytes.Repeat([]byte{7}, 32)
	s, err := New(Options{AccessToken: token})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	server := httptest.NewServer(s)
	defer server.Close()
	conn, err := transport.Dial(context.Background(), "ws"+server.URL[4:], token, "control")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	envelope := protocol.CatalogEnvelope{Version: 3, Group: string(bytes.Repeat([]byte("a"), 64)), RouteID: string(bytes.Repeat([]byte("b"), 64)), Nonce: make([]byte, 24), Ciphertext: make([]byte, 16)}
	raw, _ := protocol.EncodeCatalogEnvelope(envelope)
	request := func(message map[string]any) (int, string) {
		t.Helper()
		message["id"] = protocol.ID()
		encoded, _ := json.Marshal(message)
		if err := protocol.WriteFrame(conn, encoded, protocol.MaxControl); err != nil {
			t.Fatal(err)
		}
		count := 0
		for {
			var reply protocol.Message
			if err := protocol.ReadJSON(conn, &reply); err != nil {
				t.Fatal(err)
			}
			if reply.Type == "record" {
				count++
			}
			if reply.Type == "result" {
				return count, reply.Error
			}
		}
	}
	request(map[string]any{"type": "register", "record": json.RawMessage(raw)})
	if n, e := request(map[string]any{"type": "catalog"}); n != 1 || e != "" {
		t.Fatalf("full snapshot: %d %s", n, e)
	}
	known := map[string]string{envelope.RouteID: protocol.Digest(raw)}
	if n, e := request(map[string]any{"type": "catalog", "known": known}); n != 0 || e != "" {
		t.Fatalf("known snapshot: %d %s", n, e)
	}
	envelope.Nonce[0] = 1
	raw, _ = protocol.EncodeCatalogEnvelope(envelope)
	request(map[string]any{"type": "register", "record": json.RawMessage(raw)})
	if n, e := request(map[string]any{"type": "catalog", "known": known}); n != 1 || e != "" {
		t.Fatalf("changed ciphertext omitted: %d %s", n, e)
	}
	if n, e := request(map[string]any{"type": "catalog", "known": map[string]string{"invalid": "invalid"}}); n != 0 || e != "invalid_known_records" {
		t.Fatalf("malformed cache hint accepted: %d %s", n, e)
	}
	large := map[string]string{}
	for i := 0; i < 33; i++ {
		large[protocol.Digest([]byte{byte(i)})] = protocol.Digest(raw)
	}
	if n, e := request(map[string]any{"type": "catalog", "known": large}); n != 0 || e != "invalid_known_records" {
		t.Fatalf("unbounded cache hint accepted: %d %s", n, e)
	}
}
