# zelport-relay

Reusable Relay Protocol v1 server and standalone CLI.

The public `relay` package owns the server and listener lifecycle. Both the
standalone `cmd/zelport-relay` and Zelport App's `built_in_relay` module use this
same implementation. It depends on `github.com/zelport/zelport-sdk-go` and
provides relay transport and admission only.

Run `make build` and then `./bin/zelport-relay --help`. Admission remains explicit:

```sh
./bin/zelport-relay --listen :8090 --access-token-file ./relay-access.token
ZELPORT_RELAY_ALLOW_PUBLIC_ACCESS=true ./bin/zelport-relay --listen :8090
```

The CLI uses `ZELPORT_RELAY_*` environment names and Zelport display text. Public access only changes admission; E2E secrets are
still required by clients. Default listen address remains `127.0.0.1:8090`.

`make check` runs race tests and vet. `make check-integration` also runs SDK
client tests against a real server; see [test harness](tests/sdk-client/README.md).
Tagged dependencies are resolved directly by Go. For the integration harness,
set `ZELPORT_RELAY_VERSION` to the published relay version being tested.

## Running as root

`zelport-relay` refuses to start with effective UID 0 by default on macOS and
Linux. Run it as a regular user. To deliberately override this guard, pass
`--allow-root` or set `ZELPORT_RELAY_ALLOW_ROOT=1`. Only the exact environment value `1`
enables the exception; other values do not. An explicit `--allow-root=false`
overrides the environment. Every allowed root invocation prints a warning to
stderr, regardless of the log level. `--help` and `--version` remain available
without an exception.

This prevents accidental privileged execution; it is not a security boundary
against an administrator. The check uses the process effective UID, not sudo
environment variables, and does not detect Windows administrator privileges.

The same guard applies to `token --out`; put `--allow-root` after `token` for
that command. Prefer a dedicated unprivileged service user. A rootful Docker
daemon does not require the relay process to run as root: configure `USER` in
the image or `user:` in Compose. No automatic container exemption is applied.
The embeddable relay library does not enforce a process identity policy; the
host application owns it. Admission settings use `ZELPORT_RELAY_*` variables; the root override uses
`ZELPORT_RELAY_ALLOW_ROOT`.

Licensed under the [MIT License](LICENSE).
