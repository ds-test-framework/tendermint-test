// A replica of testcases/roundskipprevote.go
// Should adapt to the Partitioner, Partition and Part interfaces defined in partition.go to make it easier to create different partition scenarios

package roundskip

import (
	"sync"
	"time"

	"github.com/ds-test-framework/scheduler/log"
	"github.com/ds-test-framework/scheduler/testing"
	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

type roundCounter struct {
	countMap map[types.ReplicaID]int
	commits  map[int]map[types.ReplicaID]string
	faults   int
	mtx      *sync.Mutex
}

func newRoundCounter(replicas *types.ReplicaStore, faults int) *roundCounter {
	counter := &roundCounter{
		mtx:      new(sync.Mutex),
		commits:  make(map[int]map[types.ReplicaID]string),
		faults:   faults,
		countMap: make(map[types.ReplicaID]int),
	}

	for _, r := range replicas.Iter() {
		counter.countMap[r.ID] = 0
	}
	return counter
}

func (r *roundCounter) Count(msg *util.TMessageWrapper, round int) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	curRound := r.countMap[msg.From]
	_, ok := r.commits[round]
	if !ok {
		r.commits[round] = make(map[types.ReplicaID]string)
	}
	if round > curRound {
		r.countMap[msg.From] = round
	}

	if msg.Type == util.Precommit {
		precommit := msg.Msg.GetVote()
		if precommit == nil {
			return
		}
		commitValue := precommit.Vote.BlockID.Hash
		var commitS string
		if commitValue == nil {
			commitS = "nil"
		} else {
			commitS = string(commitValue)
		}

		r.commits[round][msg.From] = commitS
	}
}

func (r *roundCounter) Skipped(round int) bool {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	count := 0
	for _, rround := range r.countMap {
		if rround > round {
			count = count + 1
		}
		if count >= r.faults+1 {
			return true
		}
	}
	return false
}

type partition struct {
	honestDelayed types.ReplicaID  // 1 replica
	faults        *util.ReplicaSet // f faulty replicas
	rest          *util.ReplicaSet // 2f replicas we don't care about
}

func (p *partition) IsRest(id types.ReplicaID) bool {
	return p.rest.Exists(id)
}

func (p *partition) IsFaulty(id types.ReplicaID) bool {
	return p.faults.Exists(id)
}

func (p *partition) IsHonestDelayed(id types.ReplicaID) bool {
	return p.honestDelayed == id
}

func (p *partition) String() string {
	var str string
	str = "HonestDelayed: " + string(p.honestDelayed)
	str += "\nFaulty: "
	for _, f := range p.faults.Iter() {
		str += (string(f) + ",")
	}
	str += "\nRest: "
	for _, r := range p.rest.Iter() {
		str += (string(r) + ",")
	}
	return str
}

type replicaPartitioner struct {
	allReplicas  *types.ReplicaStore
	mtx          *sync.Mutex
	partitionMap map[int]*partition
	faults       int
	logger       *log.Logger
}

func newReplicaPartitioner(replicaStore *types.ReplicaStore, faults int, logger *log.Logger) *replicaPartitioner {
	return &replicaPartitioner{
		allReplicas:  replicaStore,
		mtx:          new(sync.Mutex),
		partitionMap: make(map[int]*partition),
		faults:       faults,
		logger:       logger,
	}
}

func (p *replicaPartitioner) NewPartition(round int) {
	// Strategy to choose the next partition comes here

	p.mtx.Lock()
	_, ok := p.partitionMap[round]
	prev, prevOk := p.partitionMap[round-1]
	p.mtx.Unlock()
	if ok {
		return
	}

	// Right now just pick the previous partition
	if prevOk {
		p.logger.With(log.LogParams{
			"partition": prev.String(),
			"round":     round,
		}).Info("Retained earlier partition")
		p.mtx.Lock()
		p.partitionMap[round] = prev
		p.mtx.Unlock()
		return
	}

	partition := &partition{
		honestDelayed: "",
		faults:        util.NewReplicaSet(),
		rest:          util.NewReplicaSet(),
	}
	for _, r := range p.allReplicas.Iter() {
		if partition.honestDelayed == "" {
			partition.honestDelayed = r.ID
		} else if partition.faults.Size() < p.faults {
			partition.faults.Add(r.ID)
		} else {
			partition.rest.Add(r.ID)
		}
	}
	p.mtx.Lock()
	p.partitionMap[round] = partition
	p.mtx.Unlock()
	p.logger.With(log.LogParams{
		"partition": partition.String(),
		"round":     round,
	}).Info("New partition")
}

func (p *replicaPartitioner) GetPartition(round int) (*partition, bool) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	partition, ok := p.partitionMap[round]
	return partition, ok
}

func (p *replicaPartitioner) GetReplicaInfo(id types.ReplicaID) *types.Replica {
	r, ok := p.allReplicas.GetReplica(id)
	if !ok {
		return nil
	}
	return r
}

type RoundSkipPrevote struct {
	testing.BaseTestCase

	allReplicas *types.ReplicaStore
	// delayedMessages maps round number to the messages that have been delayed in that round
	delayedMessages map[int][]*types.Message
	mtx             *sync.Mutex
	roundCounter    *roundCounter
	curRound        int
	faults          int
	height          int
	roundsToSkip    int

	// TODO: Should move to the `Partitioner` interface
	partitioner *replicaPartitioner
	ready       chan bool
}

func NewRoundSkipPrevote(height, roundsToSkip int) *RoundSkipPrevote {
	return &RoundSkipPrevote{
		BaseTestCase:    *testing.NewBaseTestCase("RoundSkipPrevote", 20*time.Second),
		height:          height,
		roundsToSkip:    roundsToSkip,
		curRound:        0,
		delayedMessages: make(map[int][]*types.Message),
		mtx:             new(sync.Mutex),
		ready:           make(chan bool),
	}
}

func (r *RoundSkipPrevote) Initialize(replicaStore *types.ReplicaStore, logger *log.Logger) (testing.TestCaseCtx, error) {
	faults := int((replicaStore.Size() - 1) / 3)
	r.allReplicas = replicaStore

	r.BaseTestCase.Initialize(replicaStore, logger)
	r.faults = faults
	r.partitioner = newReplicaPartitioner(replicaStore, faults, logger)
	r.partitioner.NewPartition(r.curRound)
	r.roundCounter = newRoundCounter(replicaStore, faults)
	close(r.ready)
	return r.BaseTestCase.Ctx, nil
}

func (r *RoundSkipPrevote) getAllDelayedMessages() []*types.Message {
	result := make([]*types.Message, 0)
	r.mtx.Lock()
	defer r.mtx.Unlock()

	for _, messages := range r.delayedMessages {
		result = append(result, messages...)
	}

	for k := range r.delayedMessages {
		delete(r.delayedMessages, k)
	}

	return result
}

func (r *RoundSkipPrevote) HandleMessage(msg *types.Message) (bool, []*types.Message) {

	<-r.ready
	r.mtx.Lock()
	curRound := r.curRound
	r.mtx.Unlock()

	if curRound == r.roundsToSkip {
		// Can deliver delayed messages here
		return true, r.getAllDelayedMessages()
	}

	tMsg, err := util.Unmarshal(msg.Msg)
	if err != nil {
		r.Logger.Info("Could not unmarshall message")
		return true, []*types.Message{}
	}
	r.Logger.With(log.LogParams{
		"type": tMsg.Type,
		"msg":  tMsg.Msg.String(),
		"from": msg.From,
		"to":   msg.To,
	}).Info("Handling message")

	height, round := util.ExtractHR(tMsg)
	r.Logger.With(log.LogParams{
		"height": height,
		"round":  round,
	}).Debug("Height and round of message")
	if height != r.height || round == -1 {
		return true, []*types.Message{}
	}

	// 1. Recording the round number for the replica
	r.roundCounter.Count(tMsg, round)
	if r.roundCounter.Skipped(curRound) {
		r.Logger.With(log.LogParams{
			"round": curRound,
		}).Info("Done skipping round")
		r.mtx.Lock()
		r.curRound = r.curRound + 1
		r.mtx.Unlock()
	}

	// 2. Check if message should be changed or delayed
	partition, ok := r.partitioner.GetPartition(round)
	if !ok {
		r.partitioner.NewPartition(round)
		partition, _ = r.partitioner.GetPartition(round)
	}
	r.Logger.With(log.LogParams{
		"partition": partition.String(),
	}).Debug("Partition")
	if partition.IsHonestDelayed(msg.From) {
		// Delay message based on message type
		if tMsg.Type == util.Prevote {
			r.Logger.With(log.LogParams{
				"replica": msg.From,
			}).Info("Delaying prevote")
			r.mtx.Lock()
			_, ok := r.delayedMessages[round]
			if !ok {
				r.delayedMessages[round] = make([]*types.Message, 0)
			}
			r.delayedMessages[round] = append(r.delayedMessages[round], msg)
			r.mtx.Unlock()
			return false, []*types.Message{}
		}
	} else if partition.IsFaulty(msg.From) {
		// Change vote if the vote is something we care about
		if tMsg.Type == util.Prevote { // || tMsg.Type == util.Precommit
			r.Logger.With(log.LogParams{
				"replica": msg.From,
			}).Info("Changing prevote")
			replica, ok := r.allReplicas.GetReplica(msg.From)
			if !ok {
				return true, []*types.Message{}
			}
			newVote, err := util.ChangeVote(replica, tMsg)
			if err != nil {
				// Could not change vote. This should be recoreded and retried
				return true, []*types.Message{}
			}
			newMsgB, err := util.Marshal(newVote)
			if err != nil {
				// Could not change vote. This should be recoreded and retried
				return true, []*types.Message{}
			}
			newMsg := msg.Clone()
			newMsg.ID = ""
			newMsg.Msg = newMsgB
			return false, []*types.Message{newMsg}
		}
	}

	return true, []*types.Message{}
}
