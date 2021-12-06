package sanity

import (
	"fmt"
	"time"

	"github.com/ds-test-framework/scheduler/log"
	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/testlib/handlers"
	smlib "github.com/ds-test-framework/scheduler/testlib/statemachine"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/common"
	"github.com/ds-test-framework/tendermint-test/util"
)

type threeFilters struct{}

func (threeFilters) countAndDeliver(e *types.Event, c *testlib.Context) ([]*types.Message, bool) {
	message, _ := c.GetMessage(e)
	tMsg, ok := util.GetParsedMessage(message)
	if !ok {
		return []*types.Message{}, false
	}
	faults, _ := c.Vars.GetInt("faults")
	voteCountKey := fmt.Sprintf("voteCount_%s", tMsg.To)
	if !c.Vars.Exists(voteCountKey) {
		c.Vars.SetCounter(voteCountKey)
	}
	voteCount, _ := c.Vars.GetCounter(voteCountKey)
	if voteCount.Value() >= 2*faults-1 {
		return []*types.Message{}, true
	}
	voteCount.Incr()
	return []*types.Message{message}, true
}

func (threeFilters) recordProposal(e *types.Event, c *testlib.Context) ([]*types.Message, bool) {
	message, _ := c.GetMessage(e)
	tMsg, ok := util.GetParsedMessage(message)
	if !ok {
		return []*types.Message{}, false
	}
	blockID, ok := util.GetProposalBlockIDS(tMsg)
	if ok {
		c.Vars.Set("oldProposal", blockID)
	}
	return []*types.Message{message}, true
}

func threeSetup(c *testlib.Context) {
	faults := int((c.Replicas.Cap() - 1) / 3)
	partitioner := util.NewGenericPartitioner(c.Replicas)
	partition, _ := partitioner.CreatePartition([]int{faults + 1, 2*faults - 1, 1}, []string{"toLock", "toNotLock", "faulty"})
	c.Logger().With(log.LogParams{
		"partition": partition.String(),
	}).Info("Created partition")
	c.Vars.Set("partition", partition)
}

func ThreeTestCase() *testlib.TestCase {
	filters := threeFilters{}

	sm := smlib.NewStateMachine()
	start := sm.Builder()
	start.On(common.IsCommit, smlib.FailStateLabel)
	round1 := start.On(common.RoundReached(1), "round1")
	round1.On(common.IsCommit, smlib.FailStateLabel)
	round2 := round1.On(common.RoundReached(2), "round2")
	round2.On(common.IsCommit, smlib.SuccessStateLabel)

	handler := handlers.NewHandlerCascade()
	handler.AddHandler(
		handlers.If(
			handlers.IsMessageSend().
				And(common.IsVoteFromFaulty().AsFunc()),
		).Then(common.ChangeVoteToNil),
	)
	handler.AddHandler(
		handlers.If(handlers.IsMessageSend().
			And(common.IsMessageFromRound(0).
				And(common.IsToPart("toNotLock")).
				And(common.IsMessageType(util.Prevote)).AsFunc()),
		).Then(filters.countAndDeliver),
	)
	handler.AddHandler(
		handlers.If(handlers.IsMessageSend().
			And(common.IsMessageFromRound(0).
				And(common.IsMessageType(util.Proposal)).AsFunc()),
		).Then(filters.recordProposal),
	)
	handler.AddHandler(
		handlers.If(
			handlers.IsMessageSend().
				And(common.IsMessageFromRound(1).
					And(common.IsMessageType(util.Proposal)).AsFunc()),
		).Then(handlers.DontDeliverMessage),
	)
	handler.AddHandler(smlib.NewAsyncStateMachineHandler(sm))

	testcase := testlib.NewTestCase("LockedValueCheck", 30*time.Second, handler)
	testcase.SetupFunc(common.Setup(threeSetup))
	testcase.AssertFn(func(c *testlib.Context) bool {
		commitBlock, ok := c.Vars.GetString("CommitBlockID")
		if !ok {
			return false
		}
		oldProposal, ok := c.Vars.GetString("oldProposal")
		if !ok {
			return false
		}
		curRound, ok := c.Vars.GetInt("CurRound")
		if !ok {
			return false
		}
		c.Logger().With(log.LogParams{
			"commit_blockID": commitBlock,
			"proposal":       oldProposal,
			"curRound":       curRound,
		}).Info("Checking assertion")
		return commitBlock == oldProposal && curRound == 2
	})

	return testcase
}
