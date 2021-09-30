package sanity

import (
	"errors"
	"time"

	"github.com/ds-test-framework/scheduler/testlib"
	smlib "github.com/ds-test-framework/scheduler/testlib/statemachine"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

type voteCount_One struct {
	// keeps track of the prevoted replicas for a given blockID
	recorded map[string][]string
	// keeps track of how many votes are delivered to the replicas
	// first key is for vote type and second key is for replica type
	delivered map[string]map[string]int
}

func newVoteCount_One() *voteCount_One {
	return &voteCount_One{
		recorded:  make(map[string][]string),
		delivered: make(map[string]map[string]int),
	}
}

func setup_One(c *testlib.Context) error {
	faults := int((c.Replicas.Cap() - 1) / 3)
	partitioner := util.NewStaticPartitioner(c.Replicas, faults)
	partitioner.NewPartition(0)
	partition, _ := partitioner.GetPartition(0)
	c.Vars.Set("partition", partition)
	c.Vars.Set("faults", faults)
	c.Vars.Set("voteCount", make(map[int]*voteCount_One))
	return nil
}

func getPartition(c *testlib.Context) *util.Partition {
	partition, _ := c.Vars.Get("partition")
	return partition.(*util.Partition)
}

func getVoteCount_One(c *testlib.Context, round int) *voteCount_One {
	voteCountI, _ := c.Vars.Get("voteCount")
	voteCount := voteCountI.(map[int]*voteCount_One)
	_, ok := voteCount[round]
	if !ok {
		voteCount[round] = newVoteCount_One()
	}
	return voteCount[round]
}

func setVoteCount_One(c *testlib.Context, v *voteCount_One, round int) {
	voteCountI, _ := c.Vars.Get("voteCount")
	voteCount := voteCountI.(map[int]*voteCount_One)

	voteCount[round] = v
	c.Vars.Set("voteCount", voteCount)
}

func faultyFilter_One(c *smlib.Context) ([]*types.Message, bool) {
	if !c.CurEvent.IsMessageSend() {
		return []*types.Message{}, false
	}
	messageID, _ := c.CurEvent.MessageID()
	message, ok := c.MessagePool.Get(messageID)
	if !ok {
		return []*types.Message{}, false
	}

	tMsg, err := util.Unmarshal(message.Data)
	if err != nil {
		return []*types.Message{}, false
	}
	faulty, _ := getPartition(c.Context).GetPart("faulty")

	if faulty.Contains(message.From) {
		replica, _ := c.Replicas.Get(message.From)
		newVote, err := util.ChangeVoteToNil(replica, tMsg)
		if err != nil {
			return []*types.Message{message}, true
		}
		newMsgB, err := util.Marshal(newVote)
		if err != nil {
			return []*types.Message{message}, true
		}
		return []*types.Message{c.NewMessage(message, newMsgB)}, true
	} else if faulty.Contains(message.To) {
		// Deliver everything to faulty because we need it to advance!
		return []*types.Message{message}, true
	}
	return []*types.Message{}, false
}

func round0_One(c *smlib.Context) ([]*types.Message, bool) {
	if !c.CurEvent.IsMessageSend() {
		return []*types.Message{}, false
	}
	messageID, _ := c.CurEvent.MessageID()
	message, ok := c.MessagePool.Get(messageID)
	if !ok {
		return []*types.Message{}, false
	}

	tMsg, err := util.Unmarshal(message.Data)
	if err != nil {
		return []*types.Message{}, false
	}

	if tMsg.Type != util.Prevote && tMsg.Type != util.Precommit {
		return []*types.Message{message}, true
	}

	_, round := util.ExtractHR(tMsg)

	// 1. Only keep count of messages to deliver in round 0
	if round != 0 {
		return []*types.Message{message}, true
	}
	votes := getVoteCount_One(c.Context, round)

	faultsI, _ := c.Vars.Get("faults")
	faults := faultsI.(int)

	_, ok = votes.delivered[string(tMsg.Type)]
	if !ok {
		votes.delivered[string(tMsg.Type)] = make(map[string]int)
	}
	delivered := votes.delivered[string(tMsg.Type)]
	_, ok = delivered[string(message.To)]
	if !ok {
		delivered[string(message.To)] = 0
	}
	curDelivered := delivered[string(message.To)]
	if curDelivered <= 2*faults-1 {
		votes.delivered[string(tMsg.Type)][string(message.To)] = curDelivered + 1
		setVoteCount_One(c.Context, votes, round)
		return []*types.Message{message}, true
	}

	return []*types.Message{}, true
}

func quorumCond(c *smlib.Context) bool {
	if !c.CurEvent.IsMessageSend() {
		return false
	}

	messageID, _ := c.CurEvent.MessageID()
	message, ok := c.MessagePool.Get(messageID)
	if !ok {
		return false
	}

	faulty, _ := getPartition(c.Context).GetPart("faulty")
	faultsI, _ := c.Vars.Get("faults")
	faults := faultsI.(int)
	if faulty.Contains(message.From) {
		return false
	}

	tMsg, err := util.Unmarshal(message.Data)
	if err != nil {
		return false
	}

	if tMsg.Type != util.Prevote {
		return false
	}
	_, round := util.ExtractHR(tMsg)
	blockID, _ := util.GetVoteBlockIDS(tMsg)

	votes := getVoteCount_One(c.Context, round)
	_, ok = votes.recorded[blockID]
	if !ok {
		votes.recorded[blockID] = make([]string, 0)
	}
	votes.recorded[blockID] = append(votes.recorded[blockID], string(message.From))
	setVoteCount_One(c.Context, votes, round)
	roundZeroVotes := getVoteCount_One(c.Context, 0)

	ok, err = findIntersection(votes.recorded, roundZeroVotes.recorded, faults)
	if err == errDifferentQuorum {
		c.Abort()
	}
	return ok
}

var (
	errDifferentQuorum = errors.New("different proposal")
)

func findIntersection(new, old map[string][]string, faults int) (bool, error) {
	quorumProposal := ""
	quorum := make(map[string]bool)
	for k, v := range new {
		if len(v) >= 2*faults+1 {
			quorumProposal = k
			for _, replica := range v {
				quorum[replica] = true
			}
			break
		}
	}
	oldQuorum, ok := old[quorumProposal]
	if !ok {
		return false, errDifferentQuorum
	}
	intersection := 0
	for _, replica := range oldQuorum {
		_, ok := quorum[replica]
		if ok {
			intersection++
			if intersection > faults {
				return true, nil
			}
		}
	}

	return false, nil
}

// States:
// 	1. Skip rounds by not delivering enough precommits to the replicas
// 		1.1. Ensure one faulty replica prevotes and precommits nil
// 	2. Check that in the new round there is a quorum intersection of f+1
// 		2.1 Record the votes on the proposal to check for quorum intersection (Proposal should be same in both rounds)
func OneTestCase() *testlib.TestCase {
	sm := smlib.NewStateMachine()
	sm.Builder().On(quorumCond, smlib.SuccessStateLabel)

	handler := smlib.NewAsyncStateMachineHandler(sm)
	handler.AddEventHandler(faultyFilter_One)
	handler.AddEventHandler(round0_One)

	testcase := testlib.NewTestCase("QuorumIntersection", 30*time.Second, handler)
	testcase.SetupFunc(setup_One)

	return testcase
}
