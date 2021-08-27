package util

import (
	"fmt"
	"sync"

	"github.com/ds-test-framework/scheduler/types"
)

type Part struct {
	ReplicaSet *ReplicaSet
	Label      string
}

func (p *Part) Exists(replica types.ReplicaID) bool {
	return p.ReplicaSet.Exists(replica)
}

func (p *Part) Size() int {
	return p.ReplicaSet.Size()
}

func (p *Part) String() string {
	return fmt.Sprintf("Label: %s\nMembers: %s", p.Label, p.ReplicaSet.String())
}

type Partition struct {
	Parts map[string]*Part
	mtx   *sync.Mutex
}

func NewPartition(parts ...*Part) *Partition {
	p := &Partition{
		mtx:   new(sync.Mutex),
		Parts: make(map[string]*Part),
	}

	for _, part := range parts {
		p.Parts[part.Label] = part
	}
	return p
}

func (p *Partition) GetPart(label string) (*Part, bool) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	part, ok := p.Parts[label]
	return part, ok
}

func (p *Partition) String() string {
	str := "Parts:\n"
	p.mtx.Lock()
	defer p.mtx.Unlock()
	for _, part := range p.Parts {
		str += part.String() + "\n"
	}
	return str
}

type Partitioner interface {
	NewPartition(int)
	GetPartition(int) (*Partition, bool)
}

type StaticPartitioner struct {
	allReplicas  *types.ReplicaStore
	mtx          *sync.Mutex
	partitionMap map[int]*Partition
	faults       int
}

func NewStaticPartitioner(replicaStore *types.ReplicaStore, faults int) *StaticPartitioner {
	return &StaticPartitioner{
		allReplicas:  replicaStore,
		mtx:          new(sync.Mutex),
		partitionMap: make(map[int]*Partition),
		faults:       faults,
	}
}

func (p *StaticPartitioner) NewPartition(round int) {
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
		p.mtx.Lock()
		p.partitionMap[round] = prev
		p.mtx.Unlock()
		return
	}

	honestDelayed := &Part{
		Label:      "honestDelayed",
		ReplicaSet: NewReplicaSet(),
	}
	faulty := &Part{
		Label:      "faulty",
		ReplicaSet: NewReplicaSet(),
	}
	rest := &Part{
		Label:      "rest",
		ReplicaSet: NewReplicaSet(),
	}
	for _, r := range p.allReplicas.Iter() {
		if honestDelayed.Size() == 0 {
			honestDelayed.ReplicaSet.Add(r.ID)
		} else if faulty.Size() < p.faults {
			faulty.ReplicaSet.Add(r.ID)
		} else {
			rest.ReplicaSet.Add(r.ID)
		}
	}
	partition := NewPartition(honestDelayed, faulty, rest)
	p.mtx.Lock()
	p.partitionMap[round] = partition
	p.mtx.Unlock()
}

func (p *StaticPartitioner) GetPartition(round int) (*Partition, bool) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	partition, ok := p.partitionMap[round]
	return partition, ok
}
