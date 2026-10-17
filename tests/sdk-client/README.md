# SDK client integration coverage

These client tests need both the relay server and private client state.
The `.go.txt` fixtures are compiled into a temporary SDK source tree by
`../../scripts/check-sdk-integration.sh`. Production SDK sources are unmodified.
The test-only module requires the relay; the production SDK does not depend on
it. No test is skipped and no public test hooks are added to the SDK.

From the Relay repository root, run `make check-integration`. Set
`ZELPORT_RELAY_VERSION` to the exact released relay module version under test.
The harness uses a temporary tree and removes it after completion.
