package common

import (
	"bytes"

	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/testlib/handlers"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

func ChangeVoteToNil(e *types.Event, c *testlib.Context) ([]*types.Message, bool) {
	if !e.IsMessageSend() {
		return []*types.Message{}, false
	}
	message, ok := c.GetMessage(e)
	if !ok {
		return []*types.Message{}, false
	}
	tMsg, ok := util.GetParsedMessage(message)
	if !ok {
		return []*types.Message{}, false
	}
	if tMsg.Type != util.Precommit && tMsg.Type != util.Prevote {
		return []*types.Message{}, false
	}
	valAddr, ok := util.GetVoteValidator(tMsg)
	if !ok {
		return []*types.Message{}, false
	}
	var replica *types.Replica = nil
	for _, r := range c.Replicas.Iter() {
		addr, err := util.GetReplicaAddress(r)
		if err != nil {
			continue
		}
		if bytes.Equal(addr, valAddr) {
			replica = r
			break
		}
	}
	if replica == nil {
		return []*types.Message{}, false
	}
	newVote, err := util.ChangeVoteToNil(replica, tMsg)
	if err != nil {
		return []*types.Message{}, false
	}
	msgB, err := newVote.Marshal()
	if err != nil {
		return []*types.Message{}, false
	}
	return []*types.Message{c.NewMessage(message, msgB)}, true
}

func RecordMessage(label string) handlers.HandlerFunc {
	return func(e *types.Event, c *testlib.Context) ([]*types.Message, bool) {
		message, ok := c.GetMessage(e)
		if !ok {
			return []*types.Message{}, false
		}
		c.Vars.Set(label, message)
		return []*types.Message{}, true
	}
}
