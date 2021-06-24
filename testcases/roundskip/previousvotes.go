package roundskip

import (
	"sync"

	"github.com/ds-test-framework/scheduler/log"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

// We deliver prevotes of a prior round and check the commit value.
type PreviousPrevotes struct {
	proposal        string
	delayedProposal *types.Message
	pMtx            *sync.Mutex
	RoundSkipPrevote
}

func NewPreviousVote(height, roundsToSkip int) *PreviousPrevotes {
	return &PreviousPrevotes{
		pMtx:             new(sync.Mutex),
		RoundSkipPrevote: *NewRoundSkipPrevote(height, roundsToSkip),
		delayedProposal:  nil,
	}
}

func (p *PreviousPrevotes) HandleMessage(msg *types.Message) (bool, []*types.Message) {

	ok, extra := p.RoundSkipPrevote.HandleMessage(msg)

	tMsg, err := util.Unmarshal(msg.Msg)
	if err != nil {
		p.Logger.Info("Could not unmarshall message")
		return true, []*types.Message{}
	}

	if tMsg.Type == util.Proposal {
		proposal := tMsg.Msg.GetProposal().Proposal

		p.pMtx.Lock()
		p.proposal = string(proposal.BlockID.Hash)
		p.pMtx.Unlock()

		partition, _ := p.partitioner.GetPartition(int(proposal.Round))
		honestDelayed, _ := partition.GetPart("honestDelayed")

		if honestDelayed.Exists(msg.From) {
			// Proposer is the one who's messages we have been blocking. Deliver votes from previous round

			if proposal.PolRound == -1 {
				p.Logger.With(log.LogParams{
					"pol_round": proposal.PolRound,
					"from":      msg.From,
				}).Info("Proposal POL -1 when it shouldn't be")
				return ok, extra
			}

			p.Logger.With(log.LogParams{
				"pol_round": proposal.PolRound,
				"from":      msg.From,
			}).Info("Delivering earlier prevotes")

			p.mtx.Lock()
			moreExtra, exists := p.delayedMessages[int(proposal.PolRound)]
			if exists {
				extra = append(extra, moreExtra...)
				delete(p.delayedMessages, int(proposal.PolRound))
			}
			p.mtx.Unlock()
		}
	}

	return ok, extra
}
