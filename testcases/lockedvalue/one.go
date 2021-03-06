package lockedvalue

import (
	"time"

	"github.com/ds-test-framework/scheduler/log"
	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/testlib/handlers"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

type testCaseOneFilters struct{}

func (t testCaseOneFilters) faultyReplicaFilter(e *types.Event, c *testlib.Context) ([]*types.Message, bool) {
	message, _ := c.GetMessage(e)
	tMsg, ok := util.GetParsedMessage(message)
	if !ok {
		return []*types.Message{}, false
	}
	u := commonUtil{}
	if u.isFaultyVote(e, c, tMsg) {
		return []*types.Message{u.changeFultyVote(e, c, message, tMsg)}, true
	}
	return []*types.Message{}, false
}

func (testCaseOneFilters) Round0(e *types.Event, c *testlib.Context) ([]*types.Message, bool) {
	message, _ := c.GetMessage(e)
	tMsg, ok := util.GetParsedMessage(message)
	if !ok {
		return []*types.Message{}, false
	}

	round := tMsg.Round()
	if round == -1 {
		return []*types.Message{message}, true
	}
	if round != 0 {
		return []*types.Message{}, false
	}

	switch tMsg.Type {
	case util.Proposal:
		if blockID, ok := util.GetProposalBlockIDS(tMsg); ok {
			c.Vars.Set("oldProposal", blockID)
		}
	case util.Prevote:
		partition := getReplicaPartition(c)
		honestDelayed, _ := partition.GetPart("honestDelayed")

		if honestDelayed.Contains(tMsg.From) {
			delayedPrevotesI, ok := c.Vars.Get("delayedPrevotes")
			if !ok {
				c.Vars.Set("delayedPrevotes", make([]string, 0))
				delayedPrevotesI, _ = c.Vars.Get("delayedPrevotes")
			}

			delayedPrevotes := delayedPrevotesI.([]string)
			delayedPrevotes = append(delayedPrevotes, message.ID)
			c.Vars.Set("delayedPrevotes", delayedPrevotes)
			return []*types.Message{}, true
		}
	}
	return []*types.Message{message}, true
}

func (t testCaseOneFilters) Round1(e *types.Event, c *testlib.Context) ([]*types.Message, bool) {
	message, _ := c.GetMessage(e)
	tMsg, ok := util.GetParsedMessage(message)
	if !ok {
		return []*types.Message{}, false
	}

	round := tMsg.Round()
	if round == -1 {
		return []*types.Message{message}, true
	} else if round != 1 {
		return []*types.Message{}, false
	}
	partition := getReplicaPartition(c)
	honestDelayed, _ := partition.GetPart("honestDelayed")
	replica, _ := c.Replicas.Get(tMsg.From)

	if tMsg.Type == util.Proposal {
		t.recordDelayedProposal(c, message.ID)
		return []*types.Message{}, true
	} else if tMsg.Type == util.Prevote && honestDelayed.Contains(tMsg.From) && util.IsVoteFrom(tMsg, replica) {
		voteBlockID, ok := util.GetVoteBlockIDS(tMsg)
		if ok {
			oldProposal, _ := c.Vars.GetString("oldProposal")
			if voteBlockID != oldProposal {
				c.Logger().With(log.LogParams{
					"old_proposal": oldProposal,
					"vote_blockid": voteBlockID,
				}).Info("Failing because locked value was not voted")
				c.Abort()
			}
		}
	}
	return []*types.Message{message}, true
}

func (testCaseOneFilters) recordDelayedProposal(c *testlib.Context, id string) {
	delayedProposalI, ok := c.Vars.Get("delayedProposals")
	if !ok {
		c.Vars.Set("delayedProposals", make([]string, 0))
		delayedProposalI, _ = c.Vars.Get("delayedProposals")
	}
	delayedProposal := delayedProposalI.([]string)
	delayedProposal = append(delayedProposal, id)
	c.Vars.Set("delayedProposals", delayedProposal)
}

func (testCaseOneFilters) Round2(e *types.Event, c *testlib.Context) ([]*types.Message, bool) {

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
			oldPropI, ok := c.Vars.Get("oldProposal")
			if ok {
				oldProp := oldPropI.(string)
				if oldProp == blockID {
					c.Logger().With(log.LogParams{
						"round0_proposal": oldProp,
						"round2_proposal": blockID,
					}).Info("Failing because proposals are the same! Expecting different proposals")
					c.Abort()
				}
			}
		}
	case util.Prevote:
		partition := getReplicaPartition(c)
		honestDelayed, _ := partition.GetPart("honestDelayed")
		replica, _ := c.Replicas.Get(tMsg.From)

		if honestDelayed.Contains(tMsg.From) && util.IsVoteFrom(tMsg, replica) {
			// c.Logger().Info("Checking unlocked vote")
			oldProp, _ := c.Vars.GetString("oldProposal")
			voteBlockID, ok := util.GetVoteBlockIDS(tMsg)
			c.Vars.Set("vote", voteBlockID)
			if ok && voteBlockID == oldProp {
				c.Logger().With(log.LogParams{
					"round0_proposal": oldProp,
					"vote":            voteBlockID,
				}).Info("Failing because replica did not unlock")
				c.Abort()
			}
		}
	}
	return []*types.Message{message}, true
}

func testCaseOneSetup(c *testlib.Context) error {
	faults := int((c.Replicas.Cap() - 1) / 3)
	partition, _ := util.
		NewGenericPartitioner(c.Replicas).
		CreatePartition([]int{faults, 1, 2 * faults}, []string{"faulty", "honestDelayed", "rest"})
	c.Vars.Set("partition", partition)
	c.Vars.Set("faults", faults)
	c.Logger().With(log.LogParams{
		"partition": partition.String(),
	}).Info("Partitiion created")
	return nil
}

func getReplicaPartition(c *testlib.Context) *util.Partition {
	v, _ := c.Vars.Get("partition")
	return v.(*util.Partition)
}

type testCaseOneCond struct{}

func (t testCaseOneCond) commitNewCond(e *types.Event, c *testlib.Context) bool {
	cEventType := e.Type
	switch cEventType := cEventType.(type) {
	case *types.GenericEventType:
		if cEventType.T != "Committing block" {
			return false
		}
		blockID, ok := cEventType.Params["block_id"]
		if !ok {
			return false
		}
		newProposalI, ok := c.Vars.Get("newProposal")
		if !ok {
			return false
		}
		newProposal := newProposalI.(string)
		c.Logger().With(log.LogParams{
			"new_proposal": newProposal,
			"commit_block": blockID,
		}).Info("Checking commit")
		if blockID == newProposal {
			c.EndTestCase()
			return true
		}
	}
	return false
}

func (t testCaseOneCond) commitOldCond(e *types.Event, c *testlib.Context) bool {
	cEventType := e.Type
	switch cEventType := cEventType.(type) {
	case *types.GenericEventType:
		if cEventType.T != "Committing block" {
			return false
		}
		blockID, ok := cEventType.Params["block_id"]
		if !ok {
			return false
		}
		oldProposalI, ok := c.Vars.Get("oldProposal")
		if !ok {
			return false
		}
		oldProposal := oldProposalI.(string)
		c.Logger().With(log.LogParams{
			"old_proposal": oldProposal,
			"commit_block": blockID,
		}).Info("Checking commit")
		if blockID == oldProposal {
			return true
		}
	}
	return false
}

func (testCaseOneFilters) round0Message(e *types.Event, c *testlib.Context) bool {
	tMsg, ok := util.GetMessageFromEvent(e, c)
	if !ok {
		return false
	}
	round := tMsg.Round()
	return round == 0
}

func (testCaseOneFilters) round0(e *types.Event, c *testlib.Context) ([]*types.Message, bool) {
	message, _ := c.GetMessage(e)
	tMsg, ok := util.GetParsedMessage(message)
	if !ok {
		return []*types.Message{}, false
	}

	switch tMsg.Type {
	case util.Proposal:
		if blockID, ok := util.GetProposalBlockIDS(tMsg); ok {
			c.Vars.Set("oldProposal", blockID)
		}
	case util.Prevote:
		partition := getReplicaPartition(c)
		honestDelayed, _ := partition.GetPart("honestDelayed")

		if honestDelayed.Contains(tMsg.From) {
			delayedPrevotesI, ok := c.Vars.Get("delayedPrevotes")
			if !ok {
				c.Vars.Set("delayedPrevotes", make([]string, 0))
				delayedPrevotesI, _ = c.Vars.Get("delayedPrevotes")
			}

			delayedPrevotes := delayedPrevotesI.([]string)
			delayedPrevotes = append(delayedPrevotes, message.ID)
			c.Vars.Set("delayedPrevotes", delayedPrevotes)
			return []*types.Message{}, true
		}
	}
	return []*types.Message{message}, true
}

func One() *testlib.TestCase {
	filters := testCaseOneFilters{}
	cond := testCaseOneCond{}
	commonCond := commonCond{}

	stateMachine := handlers.NewStateMachine()

	builder := stateMachine.Builder()
	round2 := builder.
		On(commonCond.valueLockedCond, "LockedValue").
		On(commonCond.roundReached(1), "Round1").
		On(commonCond.roundReached(2), "Round2")

	round2.On(cond.commitNewCond, handlers.SuccessStateLabel)
	round2.On(cond.commitOldCond, handlers.FailStateLabel)

	handler := handlers.NewHandlerCascade(
		handlers.WithStateMachine(stateMachine),
	)
	handler.AddHandler(filters.faultyReplicaFilter)
	handler.AddHandler(filters.Round0)
	handler.AddHandler(filters.Round1)
	handler.AddHandler(filters.Round2)

	testcase := testlib.NewTestCase("LockedValueOne", 50*time.Second, handler)
	testcase.SetupFunc(testCaseOneSetup)

	testcase.AssertFn(func(c *testlib.Context) bool {
		newProposal, ok := c.Vars.GetString("newProposal")
		if !ok {
			return false
		}
		vote, ok := c.Vars.GetString("vote")
		if !ok {
			return false
		}
		return vote == newProposal
	})

	return testcase
}
