package roundskip

import (
	"strconv"
	"time"

	"github.com/ds-test-framework/scheduler/testing"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

type heightCond struct {
	height int
}

func (h *heightCond) Check(e *testing.EventWrapper, _ *testing.VarSet) bool {
	t, ok := e.Event.Type.(*types.ReplicaEventType)
	if ok && t.T == "NewHeightRoundStep" {
		nh, ok := t.Params["height"]
		if ok {
			nhI, err := strconv.Atoi(nh)
			if err != nil && nhI == h.height {
				return true
			}
		}
	}
	return false
}

func newHeightCond(height int) *heightCond {
	return &heightCond{height: height}
}

type receivedProposalCond struct{}

func (r *receivedProposalCond) Check(e *testing.EventWrapper, _ *testing.VarSet) bool {
	_, ok := e.Event.MessageID()
	if ok {
		t, ok := e.Event.Type.(*types.SendMessageEventType)
		if ok {
			msg, err := util.Unmarshal(t.Message().Msg)
			if err == nil && msg.Type == util.Proposal {
				return true
			}
		}
	}
	return false
}

type roundsSkippedCond struct {
	roundsToSkip int
}

func (r *roundsSkippedCond) Check(_ *testing.EventWrapper, v *testing.VarSet) bool {
	curRoundI, ok := v.Get("curRound")
	if ok {
		curRound, ok := curRoundI.(int)
		if ok && curRound >= r.roundsToSkip {
			return true
		}
	}
	return false
}

func newRoundSkippedCond(roundsToSkip int) *roundsSkippedCond {
	return &roundsSkippedCond{
		roundsToSkip: roundsToSkip,
	}
}

type delayVotesAction struct{}

func (*delayVotesAction) Step(e *testing.EventWrapper, mPool *testing.MessagePool, vars *testing.VarSet) []*types.Message {
	// All the logic comes in here! Can probably refine it more
	return []*types.Message{}
}

type deliverDelayedAction struct{}

func (*deliverDelayedAction) Step(e *testing.EventWrapper, mPool *testing.MessagePool, vars *testing.VarSet) []*types.Message {
	rest := mPool.PickAll()
	vars.Set("delayedDelivered", true)
	return rest
}

type delayedDeliveredCond struct{}

func (*delayedDeliveredCond) Check(_ *testing.EventWrapper, v *testing.VarSet) bool {
	delayedDelivered, ok := v.Get("delayedDelivered")
	if ok {
		delayedDeliveredB, ok := delayedDelivered.(bool)
		if ok && delayedDeliveredB {
			return true
		}
	}
	return false
}

func PrevoteTestCase(height, numrounds int) *testing.TestCase {
	testCase := testing.NewTestCase("DelayPrevotes", 30*time.Second)

	waitingForProposal := testCase.CreateState("WaitingForProposal", &testing.AllowAllAction{})
	delayVotes := testCase.CreateState("DelayVotes", &delayVotesAction{})
	deliverDelayed := testCase.CreateState("DeliverDelayed", &deliverDelayedAction{})

	testCase.StartState().Upon(newHeightCond(height), waitingForProposal)
	waitingForProposal.Upon(&receivedProposalCond{}, delayVotes)
	delayVotes.Upon(newRoundSkippedCond(numrounds), deliverDelayed)
	deliverDelayed.Upon(&delayedDeliveredCond{}, testCase.SuccessState())

	return testCase
}
