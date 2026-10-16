// Package relay carries admitted, independently E2E-encrypted peer connections.
package relay

import (
	"crypto/subtle"
	"errors"
	"github.com/coder/websocket"
	"github.com/zelport/zelport-sdk-go/protocol"
	"github.com/zelport/zelport-sdk-go/secure"
	"github.com/zelport/zelport-sdk-go/transport"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

type Options struct {
	AllowUnauthenticated bool
	AccessToken          []byte
	MaxConnections       int
}
type Server struct {
	mu           sync.Mutex
	opts         Options
	closed       bool
	connections  map[net.Conn]bool
	devices      map[string]*control
	pending      map[string]*pairing
	pendingCount int
	handshakes   chan struct{}
	wg           sync.WaitGroup
}
type control struct {
	conn           net.Conn
	out            chan protocol.Message
	done           chan struct{}
	once           sync.Once
	group, id, cap string
	record         []byte
	recordDigest   string
}

func (c *control) close() { c.once.Do(func() { close(c.done); c.conn.Close() }) }
func (c *control) send(m protocol.Message) bool {
	select {
	case <-c.done:
		return false
	default:
	}
	select {
	case c.out <- m:
		return true
	default:
		c.close()
		return false
	}
}

type pairing struct {
	consumer, provider *control
	peer               protocol.RelayRoute
	cap                string
	joined             chan net.Conn
	done               chan struct{}
}

func New(o Options) (*Server, error) {
	if (!o.AllowUnauthenticated && len(o.AccessToken) != 32) || (o.AllowUnauthenticated && len(o.AccessToken) != 0) {
		return nil, errors.New("relay requires a 32-byte access token, or explicit unauthenticated mode with no token")
	}
	if o.MaxConnections == 0 {
		o.MaxConnections = 128
	}
	if o.MaxConnections < 1 || o.MaxConnections > 4096 {
		return nil, errors.New("max connections must be between 1 and 4096")
	}
	o.AccessToken = append([]byte(nil), o.AccessToken...)
	return &Server{opts: o, connections: map[net.Conn]bool{}, devices: map[string]*control{}, pending: map[string]*pairing{}, handshakes: make(chan struct{}, 16)}, nil
}
func (s *Server) Close() error {
	s.mu.Lock()
	s.closed = true
	connections := make([]net.Conn, 0, len(s.connections))
	for c := range s.connections {
		connections = append(connections, c)
	}
	s.mu.Unlock()
	for _, c := range connections {
		c.Close()
	}
	s.wg.Wait()
	return nil
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v3/ws" || r.Method != "GET" {
		http.NotFound(w, r)
		return
	}
	if len(r.Header.Values("Origin")) != 0 {
		http.Error(w, "browser connections are not supported", 403)
		return
	}
	select {
	case s.handshakes <- struct{}{}:
	default:
		http.Error(w, "handshake capacity reached", 503)
		return
	}
	held := true
	defer func() {
		if held {
			<-s.handshakes
		}
	}()
	s.mu.Lock()
	if s.closed || len(s.connections) >= s.opts.MaxConnections {
		s.mu.Unlock()
		http.Error(w, "connection capacity reached", 503)
		return
	}
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()
	ws, e := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if e != nil {
		return
	}
	raw := transport.Wrap(r.Context(), ws)
	defer raw.Close()
	s.mu.Lock()
	if s.closed || len(s.connections) >= s.opts.MaxConnections {
		s.mu.Unlock()
		return
	}
	s.connections[raw] = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.connections, raw); s.mu.Unlock() }()
	c, e := secure.Admit(r.Context(), raw, false, s.opts.AccessToken, protocol.Hello{})
	<-s.handshakes
	held = false
	if e != nil {
		return
	}
	defer c.Close()
	switch c.(*secure.Conn).Hello.Purpose {
	case "control":
		s.control(c)
	case "connect":
		s.connect(c)
	case "join":
		s.join(c)
	}
}
func (s *Server) control(conn net.Conn) {
	c := &control{conn: conn, out: make(chan protocol.Message, 64), done: make(chan struct{}), cap: protocol.RandomNonce()}
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			select {
			case <-c.done:
				return
			case m := <-c.out:
				conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
				if protocol.WriteJSON(conn, m) != nil {
					c.close()
					return
				}
			}
		}
	}()
	defer func() {
		c.close()
		<-writerDone
		s.mu.Lock()
		if c.id != "" && s.devices[c.group+"/"+c.id] == c {
			delete(s.devices, c.group+"/"+c.id)
		}
		for _, p := range s.pending {
			if p.consumer == c || p.provider == c {
				select {
				case <-p.done:
				default:
					close(p.done)
				}
			}
		}
		s.changedLocked(c.group)
		s.mu.Unlock()
	}()
	for {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		var m protocol.Message
		if protocol.ReadJSON(conn, &m) != nil {
			return
		}
		if !protocol.ValidID(m.ID) {
			return
		}
		reply := protocol.Message{Type: "result", ID: m.ID}
		switch m.Type {
		case "register", "withdraw":
			rec, e := protocol.ParseCatalogEnvelope(m.Record)
			if e != nil {
				reply.Error = "invalid_record"
				break
			}
			s.mu.Lock()
			key := rec.Group + "/" + rec.RouteID
			if c.id == "" {
				if len(s.devices) >= 32 {
					reply.Error = "device_limit"
				} else if s.devices[key] != nil {
					reply.Error = "duplicate_identity"
				} else {
					c.group, c.id = rec.Group, rec.RouteID
					s.devices[key] = c
				}
			} else if c.group != rec.Group || c.id != rec.RouteID {
				reply.Error = "identity_changed"
			}
			if reply.Error == "" {
				c.record = append([]byte(nil), m.Record...)
				c.recordDigest = protocol.Digest(m.Record)
				reply.Control = c.cap
				s.changedLocked(c.group)
			}
			s.mu.Unlock()
		case "catalog":
			if !validKnownCatalog(m.Known) {
				reply.Error = "invalid_known_records"
				break
			}
			s.mu.Lock()
			if c.id == "" {
				reply.Error = "registration_required"
			} else {
				total := 0
				for _, other := range s.devices {
					if other.group == c.group {
						total += len(other.record)
						if total > protocol.MaxSnapshot {
							reply.Error = "snapshot_limit"
							break
						}
						// Comparing opaque ciphertext requires no catalog or E2E keys.
						if m.Known[other.id] != other.recordDigest {
							c.send(protocol.Message{Type: "record", ID: m.ID, Record: other.record})
						}
					}
				}
			}
			s.mu.Unlock()
		default:
			reply.Error = "invalid_operation"
		}
		if !c.send(reply) {
			return
		}
	}
}
func (s *Server) changedLocked(group string) {
	for _, c := range s.devices {
		if c.group == group {
			c.send(protocol.Message{Type: "changed"})
		}
	}
}
func equal(a, b string) bool {
	return len(a) > 0 && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
func (s *Server) connect(conn net.Conn) {
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	var m protocol.Message
	if protocol.ReadJSON(conn, &m) != nil || m.Type != "connect" || m.Route == nil {
		return
	}
	pctx := *m.Route
	if _, e := pctx.Bytes(); e != nil {
		return
	}
	s.mu.Lock()
	consumer, provider := s.devices[pctx.Group+"/"+pctx.ConsumerRoute], s.devices[pctx.Group+"/"+pctx.ProviderRoute]
	key := pctx.Group + "/" + pctx.Session
	if consumer == nil || provider == nil || !equal(m.Control, consumer.cap) || s.pendingCount >= 32 || s.pending[key] != nil {
		s.mu.Unlock()
		protocol.WriteJSON(conn, protocol.Message{Type: "result", Error: "provider_unavailable"})
		return
	}
	p := &pairing{consumer: consumer, provider: provider, peer: pctx, cap: protocol.RandomNonce(), joined: make(chan net.Conn, 1), done: make(chan struct{})}
	s.pending[key] = p
	s.pendingCount++
	waiting := true
	provider.send(protocol.Message{Type: "offer", Route: &pctx, Capability: p.cap})
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if waiting {
			s.pendingCount--
		}
		if s.pending[key] == p {
			delete(s.pending, key)
		}
		select {
		case <-p.done:
		default:
			close(p.done)
		}
		s.mu.Unlock()
	}()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case peer := <-p.joined:
		s.mu.Lock()
		s.pendingCount--
		waiting = false
		s.mu.Unlock()
		defer peer.Close()
		if protocol.WriteJSON(conn, protocol.Message{Type: "result"}) != nil {
			return
		}
		conn.SetDeadline(time.Time{})
		peer.SetDeadline(time.Time{})
		finished := make(chan struct{})
		go func() {
			select {
			case <-consumer.done:
			case <-provider.done:
			case <-p.done:
			case <-finished:
				return
			}
			conn.Close()
			peer.Close()
		}()
		Bridge(conn, peer)
		close(finished)
	case <-p.done:
	case <-consumer.done:
	case <-timer.C:
	}
}
func (s *Server) join(conn net.Conn) {
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	var m protocol.Message
	if protocol.ReadJSON(conn, &m) != nil || m.Type != "join" || m.Route == nil {
		return
	}
	s.mu.Lock()
	p := s.pending[m.Route.Group+"/"+m.Route.Session]
	valid := p != nil && p.peer == *m.Route && equal(p.cap, m.Capability) && equal(p.provider.cap, m.Control)
	if valid {
		p.cap = ""
	}
	s.mu.Unlock()
	if !valid {
		protocol.WriteJSON(conn, protocol.Message{Type: "result", Error: "invalid_join"})
		return
	}
	if protocol.WriteJSON(conn, protocol.Message{Type: "result"}) != nil {
		return
	}
	select {
	case p.joined <- conn:
	case <-p.done:
		return
	}
	<-p.done
}

// Bridge owns both directions until one fails; bounded io.Copy buffers provide backpressure.
func Bridge(a, b net.Conn) {
	done := make(chan struct{}, 2)
	copySide := func(dst, src net.Conn) {
		io.CopyBuffer(dst, src, make([]byte, 32<<10))
		a.Close()
		b.Close()
		done <- struct{}{}
	}
	go copySide(a, b)
	go copySide(b, a)
	<-done
	<-done
}

func validKnownCatalog(known map[string]string) bool {
	if len(known) > protocol.MaxKnownCatalog {
		return false
	}
	for route, digest := range known {
		if !protocol.ValidLocator(route) || !protocol.ValidLocator(digest) {
			return false
		}
	}
	return true
}
