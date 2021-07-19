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

type ModifiedProposal struct {
	honestReplica  types.ReplicaID
	othersProposed bool
	pMtx           *sync.Mutex
	RoundSkipPrevote
}

func NewModifiedProposal(height, roundsToSkip int) *ModifiedProposal {
	return &ModifiedProposal{
		pMtx:             new(sync.Mutex),
		RoundSkipPrevote: *NewRoundSkipPrevote(height, roundsToSkip),
		othersProposed:   false,
	}
}

func (m *ModifiedProposal) HandleMessage(msg *types.Message) (bool, []*types.Message) {
	ok, extra := m.RoundSkipPrevote.HandleMessage(msg)

	tMsg, err := util.Unmarshal(msg.Msg)
	if err != nil {
		m.Logger.Info("Could not unmarshall message")
		return true, []*types.Message{}
	}

	if tMsg.Type == util.Proposal {
		proposal := tMsg.Msg.GetProposal().Proposal
		partition, _ := m.partitioner.GetPartition(int(proposal.Round))
		honestDelayed, _ := partition.GetPart("honestDelayed")

		if honestDelayed.Exists(msg.From) {
			m.pMtx.Lock()
			othersProposed := m.othersProposed
			m.pMtx.Unlock()

			if othersProposed {
				m.Logger.Info("Changing proposal")
				replica, rok := m.allReplicas.GetReplica(msg.From)
				if !rok {
					return ok, extra
				}
				changedProposal, err := util.ChangeProposal(replica, tMsg)
				if err == nil {
					msgB, err := util.Marshal(changedProposal)
					if err == nil {
						newMsg := msg.Clone()
						newMsg.ID = ""
						newMsg.Msg = msgB

						extra = append(extra, newMsg)
						extra = append(extra, m.getAllDelayedMessages()...)

						return false, extra
					} else {
						m.Logger.With(log.LogParams{
							"error": err,
						}).Error("failted to marshal proposal")
					}
				} else {
					m.Logger.With(log.LogParams{
						"error": err,
					}).Error("failted to change proposal")
				}
			}

		} else {
			m.pMtx.Lock()
			m.othersProposed = true
			m.pMtx.Unlock()
		}
	}

	return ok, extra
}
