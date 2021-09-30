package bfttime

import (
	"time"

	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

func changeVoteFilter(c *testlib.Context) []*types.Message {
	if !c.CurEvent.IsMessageSend() {
		switch cEventType := c.CurEvent.Type.(type) {
		case *types.GenericEventType:
			if cEventType.T == "Committing block" {
				c.Success()
				return []*types.Message{}
			}
		}
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

	if tMsg.Type != util.Precommit {
		return []*types.Message{message}
	}

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
	return []*types.Message{c.NewMessage(message, newMsgB)}
}

func OneTestCase() *testlib.TestCase {

	testcase := testlib.NewTestCase("BFTTimeOne", 50*time.Second, testlib.NewGenericHandler(changeVoteFilter))

	return testcase
}
