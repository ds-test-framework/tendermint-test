package sanity

import (
	"time"

	"github.com/ds-test-framework/scheduler/log"
	"github.com/ds-test-framework/scheduler/testlib"
	smlib "github.com/ds-test-framework/scheduler/testlib/statemachine"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

func round0_Three(c *smlib.Context) ([]*types.Message, bool) {
	if !c.CurEvent.IsMessageSend() {
		return []*types.Message{}, false
	}
	messageID, _ := c.CurEvent.MessageID()
	message, ok := c.MessagePool.Get(messageID)
	if !ok {
		return []*types.Message{}, false
	}

	tMsg, err := util.Unmarshal(message.Data)
	if err != nil {
		return []*types.Message{message}, false
	}

	_, round := util.ExtractHR(tMsg)
	if round == -1 {
		return []*types.Message{message}, true
	}
	if round != 0 {
		return []*types.Message{}, false
	}
	partition := getPartition(c.Context)
	toNotLock, _ := partition.GetPart("toNotLock")

	switch {
	case tMsg.Type == util.Prevote && toNotLock.Contains(message.To):
		voteCount := getVoteCount(c.Context)
		faults, _ := c.Vars.GetInt("faults")
		_, ok := voteCount[string(message.To)]
		if !ok {
			voteCount[string(message.To)] = 0
		}
		curCount := voteCount[string(message.To)]
		if curCount >= 2*faults-1 {
			return []*types.Message{}, true
		}
		voteCount[string(message.To)] = curCount + 1
		c.Vars.Set("voteCount", voteCount)
		return []*types.Message{message}, true
	case tMsg.Type == util.Proposal && round == 0:
		blockID, ok := util.GetProposalBlockIDS(tMsg)
		if ok {
			c.Vars.Set("oldProposal", blockID)
		}
		return []*types.Message{message}, true
	}

	return []*types.Message{message}, true
}

func round1_Three(c *smlib.Context) ([]*types.Message, bool) {
	if !c.CurEvent.IsMessageSend() {
		return []*types.Message{}, false
	}
	messageID, _ := c.CurEvent.MessageID()
	message, ok := c.MessagePool.Get(messageID)
	if !ok {
		return []*types.Message{}, false
	}

	tMsg, err := util.Unmarshal(message.Data)
	if err != nil {
		return []*types.Message{message}, false
	}

	_, round := util.ExtractHR(tMsg)
	if round == -1 {
		return []*types.Message{message}, true
	}
	if round != 1 {
		return []*types.Message{}, false
	}
	if tMsg.Type == util.Proposal {
		return []*types.Message{}, true
	}

	return []*types.Message{message}, true
}

func threeSetup(c *testlib.Context) error {
	faults := int((c.Replicas.Cap() - 1) / 3)
	partitioner := util.NewGenericParititioner(c.Replicas)
	partition, err := partitioner.CreateParition([]int{faults + 1, 2*faults - 1, 1}, []string{"toLock", "toNotLock", "faulty"})
	if err != nil {
		return err
	}
	c.Vars.Set("partition", partition)
	c.Vars.Set("voteCount", make(map[types.ReplicaID]int))
	c.Vars.Set("faults", faults)
	c.Vars.Set("roundCount", make(map[string]int))
	return nil
}

func commitCond_Three(c *smlib.Context) bool {
	cEventType := c.CurEvent.Type
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

func ThreeTestCase() *testlib.TestCase {

	sm := smlib.NewStateMachine()
	start := sm.Builder()
	start.On(commitCond, smlib.FailStateLabel)
	round1 := start.On(roundReached(1), "round1")
	round1.On(commitCond, smlib.FailStateLabel)
	round2 := round1.On(roundReached(2), "round2")
	round2.On(commitCond_Three, smlib.SuccessStateLabel)

	handler := smlib.NewAsyncStateMachineHandler(sm)
	handler.AddEventHandler(faultyFilter)
	handler.AddEventHandler(round0_Three)
	handler.AddEventHandler(round1_Three)

	testcase := testlib.NewTestCase("LockedValueCheck", 30*time.Second, handler)

	testcase.SetupFunc(threeSetup)

	return testcase
}
