package sanity

import (
	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/testlib/handlers"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

type commonCond struct{}

func (commonCond) roundReached(toRound int) handlers.Condition {
	return func(e *types.Event, c *testlib.Context) bool {
		tMsg, ok := util.GetMessageFromEvent(e, c)
		if !ok {
			return false
		}
		round := tMsg.Round()
		rI, ok := c.Vars.Get("roundCount")
		if !ok {
			c.Vars.Set("roundCount", make(map[string]int))
			rI, _ = c.Vars.Get("roundCount")
		}
		roundCount := rI.(map[string]int)
		cRound, ok := roundCount[string(tMsg.From)]
		if !ok {
			roundCount[string(tMsg.From)] = round
		}
		if cRound < round {
			roundCount[string(tMsg.From)] = round
		}
		c.Vars.Set("roundCount", roundCount)

		skipped := 0
		for _, r := range roundCount {
			if r >= toRound {
				skipped++
			}
		}
		if skipped == c.Replicas.Cap() {
			c.Vars.Set("CurRound", toRound)
			return true
		}
		return false
	}
}
