package relay

import (
	"context"
	"github.com/zelport/zelport-sdk-go/transport"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestRuntimeOwnsListenerAndUpgradedConnections(t *testing.T) {
	token := make([]byte, 32)
	rt, err := Prepare("127.0.0.1:0", Options{AccessToken: token})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	rt.Start()
	address := rt.Status().Listeners[0]
	res, err := http.Get("http://" + address + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatal("relay exposed administration", res.StatusCode)
	}
	c, err := transport.Dial(context.Background(), "ws://"+address, token, "control")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	rt.Close()
	rt.Close()
	c.SetReadDeadline(time.Now().Add(time.Second))
	if _, err = c.Read(make([]byte, 1)); err == nil {
		t.Fatal("upgraded connection survived shutdown")
	} else if n, ok := err.(net.Error); ok && n.Timeout() {
		t.Fatal("shutdown left upgraded connection waiting")
	}
	if c2, err := net.DialTimeout("tcp", address, time.Second); err == nil {
		c2.Close()
		t.Fatal("listener survived shutdown")
	}
}

func TestRuntimeBindFailureReleasesOtherFamily(t *testing.T) {
	occupied, err := net.Listen("tcp6", "[::]:0")
	if err != nil {
		t.Skip("IPv6 unavailable:", err)
	}
	defer occupied.Close()
	port := occupied.Addr().(*net.TCPAddr).Port
	address := net.JoinHostPort("", strconv.Itoa(port))
	rt, err := Prepare(address, Options{AccessToken: make([]byte, 32)})
	if err == nil {
		rt.Close()
		t.Fatal("IPv6 bind conflict hidden")
	}
	l, err := net.Listen("tcp4", net.JoinHostPort("", strconv.Itoa(port)))
	if err != nil {
		t.Fatal("failed prepare leaked IPv4 listener:", err)
	}
	l.Close()
}

func TestRuntimeAllInterfaces(t *testing.T) {
	rt, err := Prepare(":0", Options{AccessToken: make([]byte, 32)})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	rt.Start()
	for _, address := range rt.Status().Listeners {
		host, port, _ := net.SplitHostPort(address)
		if host == "0.0.0.0" {
			host = "127.0.0.1"
		} else {
			host = "::1"
		}
		c, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		c.Close()
	}
}
