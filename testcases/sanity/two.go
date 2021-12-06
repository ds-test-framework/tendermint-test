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

type twoFilters struct{}

func (twoFilters) countAndDeliver(e *types.Event, c *testlib.Context) ([]*types.Message, bool) {
	message, _ := c.GetMessage(e)
	tMsg, ok := util.GetParsedMessage(message)
	if !ok {
		return []*types.Message{}, false
	}
	voteCountKey := fmt.Sprintf("voteCount_%s", tMsg.To)
	faults, _ := c.Vars.GetInt("faults")
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

func (twoFilters) changeProposal(e *types.Event, c *testlib.Context) ([]*types.Message, bool) {
	message, _ := c.GetMessage(e)
	tMsg, ok := util.GetParsedMessage(message)
	if !ok {
		return []*types.Message{}, false
	}
	replica, _ := c.Replicas.Get(tMsg.From)
	newProp, err := util.ChangeProposalBlockID(replica, tMsg)
	if err != nil {
		c.Logger().With(log.LogParams{"error": err}).Error("Failed to change proposal")
		return []*types.Message{message}, true
	}
	newMsgB, err := newProp.Marshal()
	if err != nil {
		c.Logger().With(log.LogParams{"error": err}).Error("Failed to marshal changed proposal")
		return []*types.Message{message}, true
	}
	return []*types.Message{c.NewMessage(message, newMsgB)}, true
}

// States:
// 	1. Ensure replicas skip round by not delivering enough precommits
//		1.1 One replica prevotes and precommits nil
// 	2. In the next round change the proposal block value
// 	3. Replicas should prevote and precommit the earlier block and commit
func TwoTestCase() *testlib.TestCase {

	filters := twoFilters{}

	sm := smlib.NewStateMachine()
	start := sm.Builder()
	start.On(common.IsCommit, smlib.FailStateLabel)
	round1 := start.On(common.RoundReached(1), "round1")
	round1.On(common.IsCommit, smlib.SuccessStateLabel)
	round1.On(common.RoundReached(2), smlib.FailStateLabel)

	handler := handlers.NewHandlerCascade()
	handler.AddHandler(
		handlers.If(handlers.IsMessageSend().And(common.IsVoteFromFaulty().AsFunc())).
			Then(common.ChangeVoteToNil),
	)
	handler.AddHandler(
		handlers.If(
			handlers.IsMessageSend().
				And(common.IsMessageFromRound(0).
					And(common.IsMessageType(util.Precommit)).AsFunc()),
		).Then(filters.countAndDeliver),
	)
	handler.AddHandler(
		handlers.If(
			handlers.IsMessageSend().
				And(common.IsMessageFromRound(1).
					And(common.IsMessageType(util.Proposal)).AsFunc()),
		).Then(filters.changeProposal),
	)
	handler.AddHandler(smlib.NewAsyncStateMachineHandler(sm))

	testcase := testlib.NewTestCase("WrongProposal", 30*time.Second, handler)
	testcase.SetupFunc(common.Setup())
	testcase.AssertFn(func(c *testlib.Context) bool {
		committed, ok := c.Vars.GetBool("Committed")
		if !ok {
			return false
		}
		curRound, ok := c.Vars.GetInt("CurRound")
		if !ok {
			return false
		}
		c.Logger().With(log.LogParams{
			"committed": committed,
			"cur_round": curRound,
		}).Info("Checking assertion")
		return curRound == 1 && committed
	})

	return testcase
}
