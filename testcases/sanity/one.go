package sanity

import (
	"errors"
	"time"

	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

type oneVotes struct {
	// keeps track of the prevoted replicas for a given blockID
	recorded map[string][]string
	// keeps track of how many votes are delivered to the replicas
	// first key is for vote type and second key is for replica type
	delivered map[string]map[string]int
}

func newVotes() *oneVotes {
	return &oneVotes{
		recorded:  make(map[string][]string),
		delivered: make(map[string]map[string]int),
	}
}

func oneSetup(c *testlib.Context) error {
	faults := int((c.Replicas.Cap() - 1) / 3)
	partitioner := util.NewStaticPartitioner(c.Replicas, faults)
	partitioner.NewPartition(0)
	partition, _ := partitioner.GetPartition(0)
	c.Vars.Set("partition", partition)
	c.Vars.Set("faults", faults)
	c.Vars.Set("voteCount", make(map[int]*oneVotes))
	return nil
}

func getPartition(c *testlib.Context) *util.Partition {
	partition, _ := c.Vars.Get("partition")
	return partition.(*util.Partition)
}

func getOneVoteCount(c *testlib.Context, round int) *oneVotes {
	voteCountI, _ := c.Vars.Get("voteCount")
	voteCount := voteCountI.(map[int]*oneVotes)
	_, ok := voteCount[round]
	if !ok {
		voteCount[round] = newVotes()
	}
	return voteCount[round]
}

func setVoteCount(c *testlib.Context, v *oneVotes, round int) {
	voteCountI, _ := c.Vars.Get("voteCount")
	voteCount := voteCountI.(map[int]*oneVotes)

	voteCount[round] = v
	c.Vars.Set("voteCount", voteCount)
}

func oneAction(c *testlib.Context) []*types.Message {
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
		return []*types.Message{}
	}

	if tMsg.Type != util.Prevote && tMsg.Type != util.Precommit {
		return []*types.Message{message}
	}

	_, round := util.ExtractHR(tMsg)

	// 1. If its a faulty replica sending votes then change it

	votes := getOneVoteCount(c, round)
	faulty, _ := getPartition(c).GetPart("faulty")

	if faulty.Contains(message.From) {
		replica, _ := c.Replicas.Get(message.From)
		newVote, err := util.ChangeVote(replica, tMsg)
		if err != nil {
			return []*types.Message{message}
		}
		newMsgB, err := util.Marshal(newVote)
		if err != nil {
			return []*types.Message{message}
		}
		return []*types.Message{c.NewMessage(message, newMsgB)}
	} else if faulty.Contains(message.To) {
		// Deliver everything to faulty because we need it to advance!
		return []*types.Message{message}
	}

	// 2. Only keep count of messages to deliver in round 0
	if round != 0 {
		setVoteCount(c, votes, round)
		return []*types.Message{message}
	}

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
		setVoteCount(c, votes, round)
		return []*types.Message{message}
	}

	return []*types.Message{}
}

func quorumCond(c *testlib.Context) bool {
	if !c.CurEvent.IsMessageSend() {
		return false
	}

	messageID, _ := c.CurEvent.MessageID()
	message, ok := c.MessagePool.Get(messageID)
	if !ok {
		return false
	}

	faulty, _ := getPartition(c).GetPart("faulty")
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
	blockID, _ := util.GetVoteBlockID(tMsg)

	votes := getOneVoteCount(c, round)
	_, ok = votes.recorded[blockID]
	if !ok {
		votes.recorded[blockID] = make([]string, 0)
	}
	votes.recorded[blockID] = append(votes.recorded[blockID], string(message.From))
	setVoteCount(c, votes, round)
	roundZeroVotes := getOneVoteCount(c, 0)

	ok, err = findIntersection(votes.recorded, roundZeroVotes.recorded, faults)
	if err == errDifferentQuorum {
		c.Transition(testlib.FailureStateLabel)
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
	testcase := testlib.NewTestCase("QuorumIntersection", 30*time.Second)
	testcase.SetupFunc(oneSetup)

	builder := testcase.Builder()
	builder.Action(oneAction).On(quorumCond, testlib.SuccessStateLabel)

	return testcase
}
