package util

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ds-test-framework/scheduler/types"
)

type Part struct {
	ReplicaSet *ReplicaSet
	Label      string
}

func (p *Part) Contains(replica types.ReplicaID) bool {
	return p.ReplicaSet.Exists(replica)
}

func (p *Part) ContainsVal(addr []byte) bool {
	return p.ReplicaSet.ExistsVal(addr)
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

type GenericPartitioner struct {
	allReplicas *types.ReplicaStore
}

func NewGenericPartitioner(replicasStore *types.ReplicaStore) *GenericPartitioner {
	return &GenericPartitioner{
		allReplicas: replicasStore,
	}
}

func (g *GenericPartitioner) CreatePartition(sizes []int, labels []string) (*Partition, error) {
	if len(sizes) != len(labels) {
		return nil, errors.New("sizes and labels should be of same length")
	}
	totSize := 0
	parts := make([]*Part, len(sizes))
	for i, size := range sizes {
		if size <= 0 {
			return nil, errors.New("sizes have to be greater than 0")
		}
		totSize += size
		parts[i] = &Part{
			ReplicaSet: NewReplicaSet(),
			Label:      labels[i],
		}
	}
	if totSize != g.allReplicas.Cap() {
		return nil, errors.New("total size is not the same as number of replicas")
	}
	curIndex := 0
	for _, r := range g.allReplicas.Iter() {
		part := parts[curIndex]
		size := sizes[curIndex]
		if part.Size() < size {
			part.ReplicaSet.Add(r)
		} else {
			curIndex++
			part := parts[curIndex]
			part.ReplicaSet.Add(r)
		}
	}
	return NewPartition(parts...), nil
}
