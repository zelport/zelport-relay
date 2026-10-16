package relay

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// Runtime owns reserved listeners, HTTP serving and all upgraded relay connections.
// Prepare reserves sockets without accepting traffic; Start activates them after persistence.
type Runtime struct {
	mu              sync.Mutex
	server          *Server
	http            *http.Server
	listeners       []net.Listener
	reservation     *os.File
	started, closed bool
	failure         string
	once            sync.Once
	wg              sync.WaitGroup
	done            chan struct{}
}

type RuntimeStatus struct {
	State     string   `json:"state"`
	Listeners []string `json:"listeners"`
	Clients   int      `json:"clients"`
	Error     string   `json:"error,omitempty"`
}

func Prepare(address string, options Options) (*Runtime, error) {
	server, err := New(options)
	if err != nil {
		return nil, err
	}
	rt := &Runtime{server: server, done: make(chan struct{})}
	rt.http = &http.Server{Handler: rt, ReadHeaderTimeout: 5 * time.Second, MaxHeaderBytes: 16 << 10}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		rt.Close()
		return nil, err
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 0 || portNumber > 65535 {
		rt.Close()
		return nil, errors.New("relay port must be between 0 and 65535")
	}
	if portNumber != 0 {
		rt.reservation, err = reservePort(portNumber)
		if err != nil {
			rt.Close()
			return nil, err
		}
	}
	networks := []string{"tcp"}
	switch host {
	case "":
		networks = []string{"tcp4", "tcp6"}
	case "0.0.0.0":
		networks = []string{"tcp4"}
	case "::":
		networks = []string{"tcp6"}
	}
	for _, network := range networks {
		l, e := net.Listen(network, net.JoinHostPort(host, port))
		if e != nil {
			if host == "" && unsupportedFamily(e) {
				continue
			}
			rt.Close()
			return nil, fmt.Errorf("cannot listen on %s (%s): %w", address, network, e)
		}
		rt.listeners = append(rt.listeners, l)
		// Keep ephemeral test listeners on the same port across both families.
		if portNumber == 0 {
			portNumber = l.Addr().(*net.TCPAddr).Port
			port = strconv.Itoa(portNumber)
			rt.reservation, e = reservePort(portNumber)
			if e != nil {
				rt.Close()
				return nil, e
			}
		}
	}
	if len(rt.listeners) == 0 {
		rt.Close()
		return nil, errors.New("no supported IP family for relay listener")
	}
	return rt, nil
}

func unsupportedFamily(err error) bool {
	if errors.Is(err, syscall.EAFNOSUPPORT) || errors.Is(err, syscall.EPROTONOSUPPORT) || errors.Is(err, syscall.EADDRNOTAVAIL) {
		return true
	}
	// Winsock uses different numeric errors from Go's portable syscall errno constants.
	return runtime.GOOS == "windows" && (errors.Is(err, syscall.Errno(10047)) || errors.Is(err, syscall.Errno(10043)) || errors.Is(err, syscall.Errno(10049)))
}

func (r *Runtime) Start() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.started {
		return
	}
	r.started = true
	for _, l := range r.listeners {
		r.wg.Add(1)
		go func(l net.Listener) {
			defer r.wg.Done()
			err := r.http.Serve(l)
			r.mu.Lock()
			unexpected := !r.closed && err != nil
			if unexpected {
				r.failure = err.Error()
			}
			r.mu.Unlock()
			if unexpected {
				go r.Close()
			}
		}(l)
	}
}

func (r *Runtime) Done() <-chan struct{} { return r.done }

func (r *Runtime) Close() error {
	r.once.Do(func() {
		r.mu.Lock()
		r.closed = true
		server := r.server
		r.mu.Unlock()
		for _, l := range r.listeners {
			l.Close()
		}
		r.http.Close()
		server.Close()
		r.wg.Wait()
		if r.reservation != nil {
			r.reservation.Close()
		}
		close(r.done)
	})
	return nil
}

func (r *Runtime) Status() RuntimeStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	status := RuntimeStatus{State: "Stopped", Listeners: []string{}, Error: r.failure}
	if r.failure != "" {
		status.State = "Error"
	} else if r.started && !r.closed {
		status.State = "Listening"
	}
	if !r.closed {
		for _, l := range r.listeners {
			status.Listeners = append(status.Listeners, l.Addr().String())
		}
	}
	r.server.mu.Lock()
	status.Clients = len(r.server.devices)
	r.server.mu.Unlock()
	return status
}

// ServeHTTP selects a server generation without holding the runtime lock for a tunnel's lifetime.
func (r *Runtime) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	server := r.server
	r.mu.Unlock()
	server.ServeHTTP(w, req)
}

// ReplaceServer takes ownership of a validated server, retaining the listening sockets.
// The previous generation is fully closed before the replacement accepts requests.
func (r *Runtime) ReplaceServer(next *Server) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		next.Close()
		return
	}
	r.server.Close()
	r.server = next
}
