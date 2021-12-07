package sanity

import (
	"errors"
	"time"

	"github.com/ds-test-framework/scheduler/testlib"
	"github.com/ds-test-framework/scheduler/testlib/handlers"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/common"
	"github.com/ds-test-framework/tendermint-test/util"
)

type testCaseOneVoteCount struct {
	// keeps track of the prevoted replicas for a given blockID
	recorded map[string]map[string]bool
	// keeps track of how many votes are delivered to the replicas
	// first key is for vote type and second key is for replica type
	delivered map[string]map[string]int
}

func newTestCaseOneVoteCount() *testCaseOneVoteCount {
	return &testCaseOneVoteCount{
		recorded:  make(map[string]map[string]bool),
		delivered: make(map[string]map[string]int),
	}
}
func getTestCaseOneVoteCount(c *testlib.Context, round int) *testCaseOneVoteCount {
	voteCountI, _ := c.Vars.Get("voteCount")
	voteCount := voteCountI.(map[int]*testCaseOneVoteCount)
	_, ok := voteCount[round]
	if !ok {
		voteCount[round] = newTestCaseOneVoteCount()
	}
	return voteCount[round]
}

func setTestCaseOneVoteCount(c *testlib.Context, v *testCaseOneVoteCount, round int) {
	voteCountI, _ := c.Vars.Get("voteCount")
	voteCount := voteCountI.(map[int]*testCaseOneVoteCount)

	voteCount[round] = v
	c.Vars.Set("voteCount", voteCount)
}

func setupVoteCount(c *testlib.Context) {
	c.Vars.Set("voteCount", make(map[int]*testCaseOneVoteCount))
}

func getPartition(c *testlib.Context) *util.Partition {
	partition, _ := c.Vars.Get("partition")
	return partition.(*util.Partition)
}

type testCaseOneFilters struct{}

func (testCaseOneFilters) round0(e *types.Event, c *testlib.Context) ([]*types.Message, bool) {
	message, _ := c.GetMessage(e)
	tMsg, ok := util.GetParsedMessage(message)
	if !ok {
		return []*types.Message{}, false
	}
	round := tMsg.Round()

	// 1. Only keep count of messages to deliver in round 0
	if round != 0 {
		return []*types.Message{message}, true
	}

	if tMsg.Type != util.Precommit {
		return []*types.Message{message}, true
	}

	votes := getTestCaseOneVoteCount(c, round)
	faulty, _ := getPartition(c).GetPart("faulty")

	faults, _ := c.Vars.GetInt("faults")
	_, ok = votes.delivered[string(tMsg.Type)]
	if !ok {
		votes.delivered[string(tMsg.Type)] = make(map[string]int)
	}
	delivered := votes.delivered[string(tMsg.Type)]
	_, ok = delivered[string(tMsg.To)]
	if !ok {
		delivered[string(tMsg.To)] = 0
	}
	curDelivered := delivered[string(tMsg.To)]
	if faulty.Contains(tMsg.To) {
		// deliver only 2f-1 votes so that it does not commit.
		// In total it receives 2f votes and hence does not make progress until it hears 2f+1 votes of the next round
		if curDelivered < 2*faults-1 {
			votes.delivered[string(tMsg.Type)][string(tMsg.To)] = curDelivered + 1
			setTestCaseOneVoteCount(c, votes, round)
			return []*types.Message{message}, true
		}
	} else if curDelivered <= 2*faults-1 {
		votes.delivered[string(tMsg.Type)][string(tMsg.To)] = curDelivered + 1
		setTestCaseOneVoteCount(c, votes, round)
		return []*types.Message{message}, true
	}

	return []*types.Message{}, true
}

func quorumCond() handlers.Condition {
	return func(e *types.Event, c *testlib.Context) bool {
		message, ok := util.GetMessageFromEvent(e, c)
		if !ok {
			return false
		}
		faulty, _ := getPartition(c).GetPart("faulty")
		faults, _ := c.Vars.GetInt("faults")
		if faulty.Contains(message.From) {
			return false
		}

		if message.Type != util.Prevote {
			return false
		}
		round := message.Round()
		blockID, _ := util.GetVoteBlockIDS(message)

		votes := getTestCaseOneVoteCount(c, round)
		_, ok = votes.recorded[blockID]
		if !ok {
			votes.recorded[blockID] = make(map[string]bool)
		}
		votes.recorded[blockID][string(message.From)] = true
		setTestCaseOneVoteCount(c, votes, round)

		if round != 0 {
			roundZeroVotes := getTestCaseOneVoteCount(c, 0)

			ok, err := findIntersection(votes.recorded, roundZeroVotes.recorded, faults)
			if err == errDifferentQuorum {
				c.Abort()
			}
			if ok {
				c.Vars.Set("QuorumIntersection", true)
			}
			return ok
		}
		return false
	}
}

var (
	errDifferentQuorum = errors.New("different proposal")
)

func findIntersection(new, old map[string]map[string]bool, faults int) (bool, error) {
	quorumProposal := ""
	quorum := make(map[string]bool)
	for k, v := range new {
		if len(v) >= 2*faults+1 {
			quorumProposal = k
			for replica := range v {
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
	for replica := range oldQuorum {
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
	filters := testCaseOneFilters{}

	sm := handlers.NewStateMachine()
	sm.Builder().On(quorumCond(), handlers.SuccessStateLabel)

	handler := handlers.NewHandlerCascade(
		handlers.WithStateMachine(sm),
	)
	handler.AddHandler(
		handlers.If(
			handlers.IsMessageSend().And(common.IsVoteFromFaulty()),
		).Then(common.ChangeVoteToNil),
	)
	handler.AddHandler(filters.round0)

	testcase := testlib.NewTestCase("QuorumIntersection", 50*time.Second, handler)
	testcase.SetupFunc(common.Setup(setupVoteCount))
	testcase.AssertFn(func(c *testlib.Context) bool {
		i, ok := c.Vars.GetBool("QuorumIntersection")
		return ok && i
	})

	return testcase
}
