package util

import (
	"encoding/json"
	"fmt"

	"github.com/ds-test-framework/scheduler/types"
	"github.com/gogo/protobuf/proto"
	tmsg "github.com/tendermint/tendermint/proto/tendermint/consensus"
	prototypes "github.com/tendermint/tendermint/proto/tendermint/types"
	ttypes "github.com/tendermint/tendermint/types"
)

type MessageType string

const (
	NewRoundStep  MessageType = "NewRoundStep"
	NewValidBlock MessageType = "NewValidBlock"
	Proposal      MessageType = "Proposal"
	ProposalPol   MessageType = "ProposalPol"
	BlockPart     MessageType = "BlockPart"
	Vote          MessageType = "Vote"
	Prevote       MessageType = "Prevote"
	Precommit     MessageType = "Precommit"
	HasVote       MessageType = "HasVote"
	VoteSetMaj23  MessageType = "VoteSetMaj23"
	VoteSetBits   MessageType = "VoteSetBits"
	None          MessageType = "None"

	PreVoteType   prototypes.SignedMsgType = prototypes.PrevoteType
	PreCommitType prototypes.SignedMsgType = prototypes.PrecommitType
)

type TMessageWrapper struct {
	ChannelID uint16          `json:"chan_id"`
	MsgB      []byte          `json:"msg"`
	From      types.ReplicaID `json:"from"`
	To        types.ReplicaID `json:"to"`
	Type      MessageType     `json:"-"`
	Msg       *tmsg.Message   `json:"-"`
}

func Unmarshal(m []byte) (*TMessageWrapper, error) {
	var cMsg TMessageWrapper
	err := json.Unmarshal(m, &cMsg)
	if err != nil {
		return &cMsg, err
	}
	chid := cMsg.ChannelID
	if chid < 0x20 || chid > 0x23 {
		cMsg.Type = "None"
		return &cMsg, nil
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
		cMsg.Type = NewRoundStep
	case *tmsg.Message_NewValidBlock:
		cMsg.Type = NewValidBlock
	case *tmsg.Message_Proposal:
		cMsg.Type = Proposal
	case *tmsg.Message_ProposalPol:
		cMsg.Type = ProposalPol
	case *tmsg.Message_BlockPart:
		cMsg.Type = BlockPart
	case *tmsg.Message_Vote:
		v := tMsg.GetVote()
		if v == nil {
			cMsg.Type = Vote
			break
		}
		switch v.Vote.Type {
		case prototypes.PrevoteType:
			cMsg.Type = Prevote
		case prototypes.PrecommitType:
			cMsg.Type = Precommit
		default:
			cMsg.Type = Vote
		}
	case *tmsg.Message_HasVote:
		cMsg.Type = HasVote
	case *tmsg.Message_VoteSetMaj23:
		cMsg.Type = VoteSetMaj23
	case *tmsg.Message_VoteSetBits:
		cMsg.Type = VoteSetBits
	default:
		cMsg.Type = None
	}

	// log.Debug(fmt.Sprintf("Received message from: %s, with contents: %s", cMsg.From, cMsg.Msg.String()))
	return &cMsg, err
}

func Marshal(msg *TMessageWrapper) ([]byte, error) {
	msgB, err := proto.Marshal(msg.Msg)
	if err != nil {
		return nil, err
	}
	msg.MsgB = msgB

	result, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func ExtractHR(msg *TMessageWrapper) (int, int) {
	switch msg.Type {
	case NewRoundStep:
		hrs := msg.Msg.GetNewRoundStep()
		return int(hrs.Height), int(hrs.Round)
	case Proposal:
		prop := msg.Msg.GetProposal()
		return int(prop.Proposal.Height), int(prop.Proposal.Round)
	case Prevote:
		vote := msg.Msg.GetVote()
		return int(vote.Vote.Height), int(vote.Vote.Round)
	case Precommit:
		vote := msg.Msg.GetVote()
		return int(vote.Vote.Height), int(vote.Vote.Round)
	case Vote:
		vote := msg.Msg.GetVote()
		return int(vote.Vote.Height), int(vote.Vote.Round)
	case NewValidBlock:
		block := msg.Msg.GetNewValidBlock()
		return int(block.Height), int(block.Round)
	case ProposalPol:
		pPol := msg.Msg.GetProposalPol()
		return int(pPol.Height), -1
	case VoteSetMaj23:
		vote := msg.Msg.GetVoteSetMaj23()
		return int(vote.Height), int(vote.Round)
	case VoteSetBits:
		vote := msg.Msg.GetVoteSetBits()
		return int(vote.Height), int(vote.Round)
	case BlockPart:
		blockPart := msg.Msg.GetBlockPart()
		return int(blockPart.Height), int(blockPart.Round)
	}
	return -1, -1
}

func ChangeVote(replica *types.Replica, voteMsg *TMessageWrapper) (*TMessageWrapper, error) {
	privKey, err := GetPrivKey(replica)
	if err != nil {
		return nil, err
	}
	chainID, err := GetChainID(replica)
	if err != nil {
		return nil, err
	}

	if voteMsg.Type != Prevote && voteMsg.Type != Precommit {
		// Can't change vote of unknown type
		return voteMsg, nil
	}

	vote := voteMsg.Msg.GetVote().Vote
	vote.BlockID = prototypes.BlockID{
		Hash:          nil,
		PartSetHeader: prototypes.PartSetHeader{},
	}
	signBytes := ttypes.VoteSignBytes(chainID, vote)

	sig, err := privKey.Sign(signBytes)
	if err != nil {
		return nil, fmt.Errorf("could not sign vote: %s", err)
	}

	vote.Signature = sig

	voteMsg.Msg = &tmsg.Message{
		Sum: &tmsg.Message_Vote{
			Vote: &tmsg.Vote{
				Vote: vote,
			},
		},
	}

	return voteMsg, nil
}
