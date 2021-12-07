package lockedvalue

import (
	"time"

	"github.com/ds-test-framework/scheduler/log"
	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/testlib/handlers"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

type testCaseTwoFilters struct{}

func (testCaseTwoFilters) Round2(e *types.Event, c *testlib.Context) ([]*types.Message, bool) {

	message, _ := c.GetMessage(e)
	tMsg, ok := util.GetParsedMessage(message)
	if !ok {
		return []*types.Message{}, false
	}
	round := tMsg.Round()
	if round == -1 {
		return []*types.Message{message}, true
	}
	if round != 2 {
		return []*types.Message{}, false
	}

	switch tMsg.Type {
	case util.Proposal:
		blockID, ok := util.GetProposalBlockIDS(tMsg)
		if ok {
			c.Vars.Set("newProposal", blockID)
		}
	case util.Prevote:
		partition := getReplicaPartition(c)
		honestDelayed, _ := partition.GetPart("honestDelayed")
		replica, _ := c.Replicas.Get(tMsg.From)

		if honestDelayed.Contains(tMsg.From) && util.IsVoteFrom(tMsg, replica) {
			// c.Logger().Info("Checking unlocked vote")
			newProp, _ := c.Vars.GetString("newProposal")
			voteBlockID, ok := util.GetVoteBlockIDS(tMsg)
			c.Vars.Set("vote", voteBlockID)
			if ok && voteBlockID == newProp {
				c.Logger().With(log.LogParams{
					"round1_proposal": newProp,
					"vote":            voteBlockID,
				}).Info("Failing because replica did unlocked")
				c.Abort()
			}
		}
	}
	return []*types.Message{message}, true
}

func Two() *testlib.TestCase {

	commonCond := commonCond{}
	tOneCond := testCaseOneCond{}
	tOneFilters := testCaseOneFilters{}
	filters := testCaseTwoFilters{}

	stateMachine := handlers.NewStateMachine()
	builder := stateMachine.Builder()
	round2 := builder.
		On(commonCond.valueLockedCond, "LockedValue").
		On(commonCond.roundReached(1), "Round1").
		On(commonCond.roundReached(2), "Round2")

	round2.On(tOneCond.commitNewCond, handlers.SuccessStateLabel)
	round2.On(tOneCond.commitOldCond, handlers.FailStateLabel)

	handler := handlers.NewHandlerCascade(
		handlers.WithStateMachine(stateMachine),
	)
	handler.AddHandler(tOneFilters.faultyReplicaFilter)
	handler.AddHandler(tOneFilters.Round0)
	handler.AddHandler(tOneFilters.Round1)
	handler.AddHandler(filters.Round2)

	testcase := testlib.NewTestCase("LockedValueOne", 50*time.Second, handler)
	testcase.SetupFunc(testCaseOneSetup)

	testcase.AssertFn(func(c *testlib.Context) bool {
		oldProposal, ok := c.Vars.GetString("oldProposal")
		if !ok {
			return false
		}
		vote, ok := c.Vars.GetString("vote")
		if !ok {
			return false
		}
		return vote == oldProposal
	})

	return testcase
}
