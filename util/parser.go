package util

import (
	"encoding/json"

	"github.com/ds-test-framework/scheduler/types"
	"github.com/gogo/protobuf/proto"
	tmsg "github.com/tendermint/tendermint/proto/tendermint/consensus"
	ttypes "github.com/tendermint/tendermint/proto/tendermint/types"
)

type TendermintMessage struct {
	ChannelID uint16          `json:"chan_id"`
	MsgB      []byte          `json:"msg"`
	From      types.ReplicaID `json:"from"`
	To        types.ReplicaID `json:"to"`
	Type      string          `json:"-"`
	Msg       *tmsg.Message   `json:"-"`
}

func Unmarshal(m []byte) (*TendermintMessage, error) {
	var cMsg TendermintMessage
	err := json.Unmarshal(m, &cMsg)
	if err != nil {
		return &cMsg, err
	}

	msg := proto.Clone(new(tmsg.Message))
	msg.Reset()

	if err := proto.Unmarshal(cMsg.MsgB, msg); err != nil {
		// log.Debug("Error unmarshalling")
		cMsg.Type = "None"
		cMsg.Msg = nil
		return &cMsg, nil
	}

	tMsg := msg.(*tmsg.Message)
	cMsg.Msg = tMsg

	switch tMsg.Sum.(type) {
	case *tmsg.Message_NewRoundStep:
		cMsg.Type = "NewRoundStep"
	case *tmsg.Message_NewValidBlock:
		cMsg.Type = "NewValidBlock"
	case *tmsg.Message_Proposal:
		cMsg.Type = "Proposal"
	case *tmsg.Message_ProposalPol:
		cMsg.Type = "ProposalPol"
	case *tmsg.Message_BlockPart:
		cMsg.Type = "BlockPart"
	case *tmsg.Message_Vote:
		v := tMsg.GetVote()
		if v == nil {
			cMsg.Type = "Vote"
			break
		}
		switch v.Vote.Type {
		case ttypes.PrevoteType:
			cMsg.Type = "Prevote"
		case ttypes.PrecommitType:
			cMsg.Type = "Precommit"
		default:
			cMsg.Type = "Vote"
		}
	case *tmsg.Message_HasVote:
		cMsg.Type = "HasVote"
	case *tmsg.Message_VoteSetMaj23:
		cMsg.Type = "VoteSetMaj23"
	case *tmsg.Message_VoteSetBits:
		cMsg.Type = "VoteSetBits"
	default:
		cMsg.Type = "None"
	}

	// log.Debug(fmt.Sprintf("Received message from: %s, with contents: %s", cMsg.From, cMsg.Msg.String()))
	return &cMsg, err
}
