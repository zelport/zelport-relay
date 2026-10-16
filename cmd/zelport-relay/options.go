package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/zelport/zelport-sdk-go/protocol"
)

const relayHelp = `Usage: zelport-relay [options]
       zelport-relay token --out <file>

Options:
  --listen <address>            Bind address (default: 127.0.0.1:8090)
  --access-token-file <file>    Read access token from file
  --allow-public-access         Allow access without a token (default: false)
  --max-connections <count>     Connection limit (default: 128)
  --log-level <level>           info (default) or error
  --allow-root                 Allow running as root (warns on startup)
  --version                     Print version
  --help, -h                    Show help

Environment variables:
  ZELPORT_RELAY_ALLOW_ROOT      Only the exact value 1 enables root execution
  ZELPORT_RELAY_LISTEN
  ZELPORT_RELAY_ACCESS_TOKEN_FILE
  ZELPORT_RELAY_ACCESS_TOKEN
  ZELPORT_RELAY_ALLOW_PUBLIC_ACCESS
  ZELPORT_RELAY_MAX_CONNECTIONS
  ZELPORT_RELAY_LOG_LEVEL

Notes:
  - CLI options override environment variables. Choose one token source or public access.

Examples:
  ./zelport-relay --listen :8090 --access-token-file ./relay-access.token
  ZELPORT_RELAY_ALLOW_PUBLIC_ACCESS=true ./zelport-relay --listen :8090
`

type relayOptions struct {
	allowRoot bool
	listen    string
	token     []byte
	public    bool
	max       int
	level     string
}

func parseOptions(args []string, out io.Writer, lookup func(string) (string, bool)) (relayOptions, bool, error) {
	var cfg relayOptions
	f := flag.NewFlagSet("zelport-relay", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	rootEnv, _ := lookup(allowRootEnv)
	allowRoot := f.Bool("allow-root", rootEnv == "1", "allow running as root")
	f.String("listen", "127.0.0.1:8090", "listen address")
	f.String("access-token-file", "", "access token file")
	f.Bool("allow-public-access", false, "allow admission without a token")
	f.String("max-connections", "128", "connection limit")
	f.String("log-level", "info", "info or error")
	versionFlag := f.Bool("version", false, "print version")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(out, "zelport-relay %s (built %s)\n\n", version, builtAt)
			fmt.Fprint(out, relayHelp)
			return cfg, true, nil
		}
		return cfg, false, err
	}
	if *versionFlag {
		fmt.Fprintf(out, "zelport-relay %s (built %s)\n", version, builtAt)
		return cfg, true, nil
	}
	if f.NArg() != 0 {
		return cfg, false, errors.New("unexpected arguments")
	}
	cfg.allowRoot = *allowRoot
	explicit := map[string]bool{}
	f.Visit(func(v *flag.Flag) { explicit[v.Name] = true })
	value := func(name, env string) (string, bool) {
		if explicit[name] {
			return f.Lookup(name).Value.String(), true
		}
		if v, ok := lookup("ZELPORT_RELAY_" + env); ok {
			return v, true
		}
		return f.Lookup(name).DefValue, false
	}
	cfg.listen, _ = value("listen", "LISTEN")
	_, port, err := net.SplitHostPort(cfg.listen)
	if err != nil {
		return cfg, false, errors.New("listen must contain a bind address and port (for example :8090 or [::1]:8090)")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return cfg, false, errors.New("listen port must be between 1 and 65535")
	}
	public, _ := value("allow-public-access", "ALLOW_PUBLIC_ACCESS")
	if public != "true" && public != "false" {
		return cfg, false, errors.New("ZELPORT_RELAY_ALLOW_PUBLIC_ACCESS must be true or false")
	}
	cfg.public = public == "true"
	max, _ := value("max-connections", "MAX_CONNECTIONS")
	cfg.max, err = strconv.Atoi(max)
	if err != nil || cfg.max < 1 {
		return cfg, false, errors.New("max connections must be a positive integer")
	}
	cfg.level, _ = value("log-level", "LOG_LEVEL")
	if cfg.level != "info" && cfg.level != "error" {
		return cfg, false, errors.New("log level must be info or error")
	}
	file, hasFile := value("access-token-file", "ACCESS_TOKEN_FILE")
	token, hasToken := lookup("ZELPORT_RELAY_ACCESS_TOKEN")
	if hasFile && hasToken {
		return cfg, false, errors.New("access token file and ZELPORT_RELAY_ACCESS_TOKEN cannot be combined")
	}
	if cfg.public {
		if hasFile || hasToken {
			return cfg, false, errors.New("public access and an access token source cannot be combined")
		}
		return cfg, false, nil
	}
	if !hasFile && !hasToken {
		return cfg, false, errors.New("access token is required: use --access-token-file, ZELPORT_RELAY_ACCESS_TOKEN_FILE or ZELPORT_RELAY_ACCESS_TOKEN; use --allow-public-access only for explicit public admission")
	}
	if hasFile {
		if strings.TrimSpace(file) == "" {
			return cfg, false, errors.New("access token file path must not be empty")
		}
		b, err := os.ReadFile(file)
		if err != nil {
			return cfg, false, errors.New("cannot read access token file")
		}
		token = string(b)
	}
	cfg.token, err = protocol.DecodeKey(strings.TrimSpace(token))
	if err != nil {
		return cfg, false, errors.New("access token must contain one 32-byte Base64url token")
	}
	return cfg, false, nil
}
