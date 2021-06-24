package roundskip

import (
	"fmt"
	"sync"

	"github.com/ds-test-framework/scheduler/types"
	"github.com/ds-test-framework/tendermint-test/util"
)

type Part struct {
	ReplicaSet *util.ReplicaSet
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
