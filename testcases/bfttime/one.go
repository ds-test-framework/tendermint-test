package bfttime

import (
	"fmt"
	"time"

	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/types"
	sutil "github.com/ds-test-framework/scheduler/util"
	"github.com/ds-test-framework/tendermint-test/util"
)

func getAction() testlib.StateAction {
	return func(c *testlib.Context) []*types.Message {

		if !c.CurEvent.IsMessageSend() {
			return []*types.Message{}
		}
		messageID, _ := c.CurEvent.MessageID()
		message, ok := c.MessagePool.Get(messageID)
		if !ok {
			return []*types.Message{}
		}

		tMsg, err := util.Unmarshal(message.Data)
		if err != nil {
			return []*types.Message{message}
		}

		if tMsg.Type == util.Precommit {
			curTime, _ := util.GetVoteTime(tMsg)
			maxVoteTimeI, ok := c.Vars.Get("maxVoteTime")
			if !ok {
				c.Vars.Set("maxVoteTime", curTime)
				maxVoteTimeI, _ = c.Vars.Get("maxVoteTime")
			}
			maxVoteTime := maxVoteTimeI.(time.Time)
			if curTime.After(maxVoteTime) {
				c.Vars.Set("maxVoteTime", curTime)
				maxVoteTime = curTime
			}

			replica, ok := c.Replicas.Get(message.From)
			if !ok {
				return []*types.Message{message}
			}
			newVote, err := util.ChangeVoteTime(replica, tMsg, maxVoteTime.Add(24*time.Hour))
			if err != nil {
				return []*types.Message{message}
			}
			newMsgB, err := util.Marshal(newVote)
			if err != nil {
				return []*types.Message{message}
			}

			newMsg := message.Clone().(*types.Message)
			counter := getCounter(c)
			newMsg.ID = fmt.Sprintf("%s_%s_change%d", newMsg.From, newMsg.To, counter.Next())
			newMsg.Data = newMsgB
			return []*types.Message{newMsg}
		}
		return []*types.Message{}
	}
}

func getCounter(c *testlib.Context) *sutil.Counter {
	v, _ := c.Vars.Get("counter")
	return v.(*sutil.Counter)
}

func setup(c *testlib.Context) error {
	c.Vars.Set("counter", sutil.NewCounter())
	return nil
}

func commitCond(c *testlib.Context) bool {
	cEventType := c.CurEvent.Type
	switch cEventType := cEventType.(type) {
	case *types.GenericEventType:
		if cEventType.T != "Committing block" {
			return false
		}
		return true
	}
	return false
}

func OneTestCase() *testlib.TestCase {
	testcase := testlib.NewTestCase("BFTTimeOne", 50*time.Second)
	testcase.SetupFunc(setup)

	builder := testcase.Builder().Action(getAction())
	builder.On(commitCond, testlib.SuccessStateLabel)

	return testcase
}
