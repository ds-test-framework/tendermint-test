# tendermint-test
Testing tendermint using the framework [Scheduler Test Server](https://github.com/ds-test-framework/scheduler/blob/master/docs/testserver.md)

## Organization
- [`util`](./util) is used to parse the byte array of message and convert it to `tendermint` message and to change `Vote` signatures using replica private keys
- [`testcases`](./testcases) describe the test scenarios
- [`server.go`](./server.go) instantiates a testing server and runs it.

## Development
- Update the `ServerConfig` in `main.go` and run `go run main.go` to start the testing process. The `tendermint` cluster should be run separately and configured to talk to this testing server (the same as for scheduler)

## Scenarios being tested
### RoundSkip using BlockParts
`BlockPart` messages are delayed to a subset of replicas in order to force enough replicas to move to the next round in turn forcing all replicas to move to the next round.

### RoundSkip using Prevote
`Prevote` messages are tampered and delayed in order to force all replicas to move to the next round.