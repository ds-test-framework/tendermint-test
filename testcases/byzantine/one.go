package byzantine

import (
	"time"

	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/testlib/handlers"
	smlib "github.com/ds-test-framework/scheduler/testlib/statemachine"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

func getRoundCond(toRound int) handlers.Condition {
	return func(e *types.Event, c *testlib.Context) bool {
		if !e.IsMessageSend() {
			return false
		}
		messageID, _ := e.MessageID()
		message, ok := c.MessagePool.Get(messageID)
		if !ok {
			return false
		}

		tMsg, err := util.Unmarshal(message.Data)
		if err != nil {
			return false
		}
		round := tMsg.Round()
		rI, ok := c.Vars.Get("roundCount")
		if !ok {
			c.Vars.Set("roundCount", map[string]int{})
			rI, _ = c.Vars.Get("roundCount")
		}
		roundCount := rI.(map[string]int)
		cRound, ok := roundCount[string(message.From)]
		if !ok {
			roundCount[string(message.From)] = round
		}
		if cRound < round {
			roundCount[string(message.From)] = round
		}
		c.Vars.Set("roundCount", roundCount)

		skipped := 0
		for _, r := range roundCount {
			if r >= toRound {
				skipped++
			}
		}
		return skipped == c.Replicas.Cap()
	}
}

func changeProposal(e *types.Event, c *testlib.Context) ([]*types.Message, bool) {
	return []*types.Message{}, false
}

func changeVote(e *types.Event, c *testlib.Context) ([]*types.Message, bool) {
	return []*types.Message{}, false
}

func changeBlockParts(e *types.Event, c *testlib.Context) ([]*types.Message, bool) {
	return []*types.Message{}, false
}

func One() *testlib.TestCase {
	stateMachine := smlib.NewStateMachine()

	h := handlers.NewHandlerCascade()
	h.AddHandler(changeProposal)
	h.AddHandler(changeVote)
	h.AddHandler(changeBlockParts)
	h.AddHandler(smlib.NewAsyncStateMachineHandler(stateMachine))

	testcase := testlib.NewTestCase(
		"CompetingProposals",
		30*time.Second,
		h,
	)
	return testcase
}
