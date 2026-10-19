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

Public access changes admission only; clients still require E2E secrets. The
default listen address is `127.0.0.1:8090`.

`make check` runs race tests and vet. `make check-integration` also runs SDK
client tests against a real server; see [test harness](tests/sdk-client/README.md).
Tagged dependencies are resolved directly by Go. For the integration harness,
set `ZELPORT_RELAY_VERSION` to the published relay version being tested.

## Running as root

On macOS and Linux, `zelport-relay` refuses effective UID 0 by default. Run it as
a regular user. Deliberate overrides use `--allow-root` or the exact environment
value `ZELPORT_RELAY_ALLOW_ROOT=1`; `--allow-root=false` takes precedence. Put
the flag after `token` when using `token --out`.

Every allowed root invocation warns on stderr. This guard prevents accidental
privileged execution; it is not a security boundary. The embeddable library
leaves process-identity policy to its host application.

Licensed under the [MIT License](LICENSE).
