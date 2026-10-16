package relay

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPortReservationAcrossAddresses(t *testing.T) {
	rt, err := Prepare("127.0.0.1:0", Options{AccessToken: make([]byte, 32)})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	_, port, _ := net.SplitHostPort(rt.Status().Listeners[0])
	for _, host := range []string{"", "0.0.0.0", "::1"} {
		other, err := Prepare(net.JoinHostPort(host, port), Options{AccessToken: make([]byte, 32)})
		if err == nil {
			other.Close()
			t.Fatalf("second relay accepted on %s", host)
		}
		if !strings.Contains(err.Error(), "reserved by another Zelport relay") {
			t.Fatal(err)
		}
	}
	rt.Close()
	next, err := Prepare(net.JoinHostPort("127.0.0.1", port), Options{AccessToken: make([]byte, 32)})
	if err != nil {
		t.Fatal("reservation survived close:", err)
	}
	next.Close()
}

func TestExplicitWildcardFamily(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "::"} {
		t.Run(host, func(t *testing.T) {
			rt, err := Prepare(net.JoinHostPort(host, "0"), Options{AccessToken: make([]byte, 32)})
			if err != nil {
				if host == "::" && unsupportedFamily(err) {
					t.Skip(err)
				}
				t.Fatal(err)
			}
			defer rt.Close()
			addresses := rt.Status().Listeners
			if len(addresses) != 1 {
				t.Fatal(addresses)
			}
			got, _, _ := net.SplitHostPort(addresses[0])
			if got != host {
				t.Fatalf("wanted %s, got %s", host, got)
			}
		})
	}
}

func TestPortReservationProcess(t *testing.T) {
	if address := os.Getenv("ZELPORT_TEST_LOCK_CHILD"); address != "" {
		rt, err := Prepare(address, Options{AccessToken: make([]byte, 32)})
		if err != nil {
			t.Fatal(err)
		}
		defer rt.Close()
		fmt.Println("READY")
		bufio.NewReader(os.Stdin).ReadByte()
		return
	}
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPortReservationProcess$")
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "HOME=") && !strings.HasPrefix(entry, "XDG_CACHE_HOME=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	childHome := t.TempDir()
	cmd.Env = append(cmd.Env, "HOME="+childHome, "XDG_CACHE_HOME="+filepath.Join(childHome, "cache"), "ZELPORT_TEST_LOCK_CHILD="+net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	scan := bufio.NewScanner(out)
	if !scan.Scan() || scan.Text() != "READY" {
		t.Fatal("child did not become ready", scan.Text())
	}
	other, err := Prepare(net.JoinHostPort("", strconv.Itoa(port)), Options{AccessToken: make([]byte, 32)})
	if err == nil {
		other.Close()
		t.Fatal("cross-process reservation not enforced")
	}
	if !strings.Contains(err.Error(), "reserved by another Zelport relay") {
		t.Fatal(err)
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	next, err := Prepare(net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), Options{AccessToken: make([]byte, 32)})
	if err != nil {
		t.Fatal("crash did not release reservation:", err)
	}
	next.Close()
}

func TestPortReservationConcurrentPrepare(t *testing.T) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	l.Close()
	type result struct {
		rt  *Runtime
		err error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for _, host := range []string{"127.0.0.1", ""} {
		go func(host string) {
			<-start
			rt, err := Prepare(net.JoinHostPort(host, port), Options{AccessToken: make([]byte, 32)})
			results <- result{rt, err}
		}(host)
	}
	close(start)
	winners := 0
	var winner *Runtime
	defer func() {
		if winner != nil {
			winner.Close()
		}
	}()
	for range 2 {
		r := <-results
		if r.err == nil {
			winners++
			if winner != nil {
				r.rt.Close()
			} else {
				winner = r.rt
			}
		} else if !strings.Contains(r.err.Error(), "reserved by another Zelport relay") {
			t.Error(r.err)
		}
	}
	if winners != 1 {
		t.Fatalf("got %d successful reservations", winners)
	}
}

func TestPortReservationReleaseAfterBindFailure(t *testing.T) {
	occupied, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := occupied.Addr().String()
	rt, err := Prepare(address, Options{AccessToken: make([]byte, 32)})
	if err == nil {
		rt.Close()
		occupied.Close()
		t.Fatal("occupied socket accepted")
	}
	occupied.Close()
	rt, err = Prepare(address, Options{AccessToken: make([]byte, 32)})
	if err != nil {
		t.Fatal("failed bind retained reservation:", err)
	}
	rt.Close()
}

func TestPortReservationImmediateRestartAfterTraffic(t *testing.T) {
	rt, err := Prepare("127.0.0.1:0", Options{AccessToken: make([]byte, 32)})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	rt.Start()
	address := rt.Status().Listeners[0]
	c, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err = c.Write([]byte("GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	if _, err = bufio.NewReader(c).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	rt.Close()
	c.Close()
	next, err := Prepare(address, Options{AccessToken: make([]byte, 32)})
	if err != nil {
		t.Fatal("immediate restart failed:", err)
	}
	next.Close()
}
