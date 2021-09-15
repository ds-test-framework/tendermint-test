package sanity

import (
	"time"

	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

type votes struct {
	blocks map[string]map[string]bool
}

func setup(c *testlib.Context) error {
	faults := int((c.Replicas.Cap() - 1) / 3)
	partitioner := util.NewStaticPartitioner(c.Replicas, faults)
	partitioner.NewPartition(0)
	partition, _ := partitioner.GetPartition(0)
	c.Vars.Set("paritioner", partition)
	c.Vars.Set("faults", faults)
	c.Vars.Set("voteCount", make(map[int]*votes))
	return nil
}

func oneAction(c *testlib.Context) []*types.Message {
	return []*types.Message{}
}

// States:
// 	1. Skip rounds by not delivering enough precommits to the replicas
// 		1.1. Ensure one faulty replica prevotes and precommits nil
// 	2. Check that in the new round there is a quorum intersection of f+1
// 		2.1 Record the votes on the proposal to check for quorum intersection (Proposal should be same in both rounds)
func OneTestCase() *testlib.TestCase {
	testcase := testlib.NewTestCase("QuorumIntersection", 30*time.Second)
	testcase.SetupFunc(setup)

	return testcase
}
