package lockedvalue

import (
	"time"

	"github.com/ds-test-framework/scheduler/log"
	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/testlib/handlers"
	smlib "github.com/ds-test-framework/scheduler/testlib/statemachine"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

// Does not work need to fix the handlers
var (
	stateLockedValue = "lockedValue"
	stateRound1      = "round1"
	stateForceRelock = "forceRelock"
	// stateRelocked    = "relocked"
)

type testCaseThreeFilters struct{}

func (testCaseThreeFilters) faultyVoteFilter(e *types.Event, c *testlib.Context) ([]*types.Message, bool) {
	message, _ := c.GetMessage(e)
	tMsg, ok := util.GetParsedMessage(message)
	if !ok {
		return []*types.Message{}, false
	}
	if tMsg.Type != util.Precommit && tMsg.Type != util.Prevote {
		return []*types.Message{}, false
	}

	partition := getReplicaPartition(c)
	faulty, _ := partition.GetPart("faulty")
	// honestDelayed, _ := partition.GetPart("honestDelayed")
	faultyReplica, _ := c.Replicas.Get(tMsg.From)
	if !faulty.Contains(tMsg.From) {
		return []*types.Message{}, false
	}

	// Need to fix this
	// if c.StateMachine.CurState().Is(stateForceRelock) {
	// if honestDelayed.Contains(tMsg.To) {
	// 	newPropBlockIDI, _ := c.Vars.Get("newPropBlockID")
	// 	newPropBlockID := newPropBlockIDI.(*ttypes.BlockID)
	// 	newVote, err := util.ChangeVote(faultyReplica, tMsg, newPropBlockID)
	// 	if err != nil {
	// 		return []*types.Message{}, false
	// 	}
	// 	newMsgB, err := newVote.Marshal()
	// 	if err != nil {
	// 		return []*types.Message{}, false
	// 	}
	// 	return []*types.Message{c.NewMessage(tMsg.Message, newMsgB)}, true
	// }
	// }

	newVote, err := util.ChangeVoteToNil(faultyReplica, tMsg)
	if err != nil {
		return []*types.Message{}, false
	}
	newMsgB, err := newVote.Marshal()
	if err != nil {
		return []*types.Message{}, false
	}
	return []*types.Message{c.NewMessage(message, newMsgB)}, true
}

func (testCaseThreeFilters) round0(e *types.Event, c *testlib.Context) ([]*types.Message, bool) {
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
	if tMsg.Type == util.Proposal {
		blockID, ok := util.GetProposalBlockIDS(tMsg)
		if ok {
			c.Vars.Set("oldProposal", blockID)
		}
		return []*types.Message{message}, true
	}

	partition := getReplicaPartition(c)
	honestDelayed, _ := partition.GetPart("honestDelayed")

	if honestDelayed.Contains(tMsg.From) && tMsg.Type == util.Prevote {
		return []*types.Message{}, true
	}
	return []*types.Message{message}, true
}

func (testCaseThreeFilters) higherRound(e *types.Event, c *testlib.Context) ([]*types.Message, bool) {
	message, _ := c.GetMessage(e)
	tMsg, ok := util.GetParsedMessage(message)
	if !ok {
		return []*types.Message{}, false
	}
	round := tMsg.Round()
	if round == -1 {
		return []*types.Message{message}, true
	}
	if round == 0 {
		return []*types.Message{}, false
	}

	if tMsg.Type != util.Proposal {
		return []*types.Message{message}, true
	}

	// Need to figure this out
	// curState := c.StateMachine.CurState()
	// if curState.Is(stateForceRelock) {
	// 	return []*types.Message{}, false
	// }

	blockID, ok := util.GetProposalBlockID(tMsg)
	if !ok {
		return []*types.Message{}, false
	}
	blockIDS := blockID.Hash.String()
	oldProposal, _ := c.Vars.GetString("oldProposal")

	if blockIDS != oldProposal {
		c.Vars.Set("newPropBlockID", blockID)
		c.Vars.Set("newProposal", blockIDS)
		return []*types.Message{message}, true
	}

	return []*types.Message{}, true
}

type testCaseThreeCond struct{}

func (testCaseThreeCond) diffProposal(e *types.Event, c *testlib.Context) bool {
	tMsg, ok := util.GetMessageFromEvent(e, c)
	if !ok {
		return false
	}

	if tMsg.Type != util.Proposal {
		return false
	}
	blockIDS, ok := util.GetProposalBlockIDS(tMsg)
	if !ok {
		return false
	}
	oldProposal, _ := c.Vars.GetString("oldProposal")
	if blockIDS != oldProposal {
		blockID, ok := util.GetProposalBlockID(tMsg)
		if ok {
			c.Vars.Set("newPropBlockID", blockID)
		}
		c.Vars.Set("newProposal", blockIDS)
		return true
	}
	return false
}

func (testCaseThreeCond) nextRound(e *types.Event, c *testlib.Context) bool {
	tMsg, ok := util.GetMessageFromEvent(e, c)
	if !ok {
		return false
	}
	round := tMsg.Round()
	rI, ok := c.Vars.Get("nextRoundCount")
	if !ok {
		c.Vars.Set("nextRoundCount", map[string]int{})
		rI, _ = c.Vars.Get("nextRoundCount")
	}
	nextRoundCount := rI.(map[string]int)
	cRound, ok := nextRoundCount[string(tMsg.From)]
	if !ok || cRound < round {
		nextRoundCount[string(tMsg.From)] = round
	}
	c.Vars.Set("nextRoundCount", nextRoundCount)

	curRound, _ := c.Vars.GetInt("curRound")

	skipped := 0
	for _, r := range nextRoundCount {
		if r > curRound {
			skipped++
		}
	}
	if skipped == c.Replicas.Cap() {
		c.Logger().Info("Reached next round")
		return true
	}
	return false
}

func (testCaseThreeCond) oldVote(e *types.Event, c *testlib.Context) bool {
	tMsg, ok := util.GetMessageFromEvent(e, c)
	if !ok {
		return false
	}
	if tMsg.Type != util.Precommit {
		return false
	}
	partition := getReplicaPartition(c)
	honestDelayed, _ := partition.GetPart("honestDelayed")
	replica, _ := c.Replicas.Get(tMsg.From)

	if !honestDelayed.Contains(tMsg.From) || !util.IsVoteFrom(tMsg, replica) {
		return false
	}

	oldProposal, _ := c.Vars.GetString("oldProposal")

	blockID, ok := util.GetVoteBlockIDS(tMsg)
	if !ok {
		return false
	}
	c.Vars.Set("blockVote", blockID)
	c.Logger().With(log.LogParams{
		"cur_vote":     blockID,
		"old_proposal": oldProposal,
	}).Info("Checking vote == old proposal")
	return blockID == oldProposal
}

func (testCaseThreeCond) newVote(e *types.Event, c *testlib.Context) bool {
	tMsg, ok := util.GetMessageFromEvent(e, c)
	if !ok {
		return false
	}
	if tMsg.Type != util.Precommit {
		return false
	}
	partition := getReplicaPartition(c)
	honestDelayed, _ := partition.GetPart("honestDelayed")
	replica, _ := c.Replicas.Get(tMsg.From)

	if !honestDelayed.Contains(tMsg.From) || !util.IsVoteFrom(tMsg, replica) {
		return false
	}
	newProposal, _ := c.Vars.GetString("newProposal")

	blockID, ok := util.GetVoteBlockIDS(tMsg)
	if !ok {
		return false
	}
	c.Vars.Set("blockVote", blockID)
	c.Logger().With(log.LogParams{
		"cur_vote":     blockID,
		"new_proposal": newProposal,
	}).Info("Checking vote == new proposal")
	if blockID == newProposal {
		c.EndTestCase()
		return true
	}
	return false
}

func testCaseThreeSetup(c *testlib.Context) error {
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

func Three() *testlib.TestCase {

	filters := testCaseThreeFilters{}
	cond := testCaseThreeCond{}
	commonConds := commonCond{}

	sm := smlib.NewStateMachine()
	relocked := sm.Builder().
		On(commonConds.valueLockedCond, stateLockedValue).
		On(commonConds.roundReached(1), stateRound1).
		On(cond.diffProposal, stateForceRelock)

	relocked.On(cond.oldVote, smlib.FailStateLabel)
	relocked.On(cond.newVote, smlib.SuccessStateLabel)

	handler := handlers.NewHandlerCascade()
	handler.AddHandler(filters.faultyVoteFilter)
	handler.AddHandler(filters.round0)
	handler.AddHandler(filters.higherRound)
	handler.AddHandler(smlib.NewAsyncStateMachineHandler(sm))

	testcase := testlib.NewTestCase("ChangeLockedValue", 70*time.Second, handler)
	testcase.SetupFunc(testCaseThreeSetup)
	testcase.AssertFn(func(c *testlib.Context) bool {
		return sm.CurState().Is(smlib.SuccessStateLabel)
	})
	return testcase
}
