module github.com/ds-test-framework/tendermint-test

go 1.16

replace (
    github.com/ds-test-framework/scheduler v1.9.0 => /Users/srinidhi/go/src/github.com/ds-test-framework/scheduler
)

require (
	github.com/ds-test-framework/scheduler v1.9.0
	github.com/gogo/protobuf v1.3.2
	github.com/tendermint/tendermint v0.34.10
)
