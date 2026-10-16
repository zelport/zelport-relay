// Command zelport-relay runs a token-protected or explicitly public relay.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/zelport/zelport-relay/relay"
	"github.com/zelport/zelport-sdk-go/protocol"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var version = "dev"
var builtAt = "unknown"

func main() {
	if e := run(os.Args[1:], os.Stdout); e != nil {
		fmt.Fprintln(os.Stderr, "zelport-relay:", e)
		os.Exit(1)
	}
}
func run(args []string, out io.Writer) error {
	if len(args) > 0 && args[0] == "token" {
		f := flag.NewFlagSet("token", flag.ContinueOnError)
		f.SetOutput(io.Discard)
		allowRoot := f.Bool("allow-root", os.Getenv(allowRootEnv) == "1", "allow running as root")
		path := f.String("out", "", "token output file")
		if e := f.Parse(args[1:]); e != nil {
			if errors.Is(e, flag.ErrHelp) {
				fmt.Fprintf(out, "zelport-relay %s (built %s)\n\n", version, builtAt)
				fmt.Fprintln(out, "Usage: zelport-relay token --out <file>\n\nCreates a new owner-only token file without overwriting an existing file. The token is never printed.\n\n  --allow-root  Allow running as root (warns on startup)\n  Env: ZELPORT_RELAY_ALLOW_ROOT=1 (only the exact value 1 enables it).")
				return nil
			}
			return e
		}
		if *path == "" || f.NArg() != 0 {
			return errors.New("token requires --out <file>")
		}
		if e := checkRoot(effectiveUID(), *allowRoot, os.Stderr); e != nil {
			return e
		}
		token, e := protocol.Token()
		if e != nil {
			return e
		}
		file, e := os.OpenFile(*path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		defer file.Close()
		if _, e = file.WriteString(token + "\n"); e != nil {
			return e
		}
		return file.Sync()
	}
	cfg, done, e := parseOptions(args, out, os.LookupEnv)
	if e != nil || done {
		return e
	}
	if e := checkRoot(effectiveUID(), cfg.allowRoot, os.Stderr); e != nil {
		return e
	}
	runtime, e := relay.Prepare(cfg.listen, relay.Options{AccessToken: cfg.token, AllowUnauthenticated: cfg.public, MaxConnections: cfg.max})
	if e != nil {
		return e
	}
	defer runtime.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runtime.Start()
	if cfg.level == "info" {
		fmt.Fprintf(out, "zelport-relay %s (built %s)\n", version, builtAt)
	}
	if cfg.public {
		fmt.Fprintln(out, "zelport-relay access: public (no access token required)")
	} else if cfg.level == "info" {
		fmt.Fprintln(out, "zelport-relay access: protected (access token required)")
	}
	if cfg.level == "info" {
		fmt.Fprintln(out, "zelport-relay listening on", strings.Join(runtime.Status().Listeners, ", "))
	}
	select {
	case <-runtime.Done():
		if failure := runtime.Status().Error; failure != "" {
			return errors.New(failure)
		}
	case <-ctx.Done():
		runtime.Close()
	}
	return nil
}
