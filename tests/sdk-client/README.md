# SDK white-box integration coverage

These original client tests need both the relay server and private client state.
The `.go.txt` fixtures are compiled into a temporary SDK source tree by
`../../scripts/check-sdk-integration.sh`. Production SDK sources are unmodified.
The test-only module requires the relay; the production SDK does not depend on
it. No test is skipped and no public test hooks are added to the SDK.

From the staging root, run `make check-integration`. After publishing independent
repositories, set `ZELPORT_RELAY_VERSION` to the relay version under test before
running the script. Benchmarks accept the usual Go test flags.
