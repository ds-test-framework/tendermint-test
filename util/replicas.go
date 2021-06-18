package util

import (
	"encoding/base64"
	"errors"
	"sync"

	"github.com/ds-test-framework/scheduler/types"
	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/crypto/secp256k1"
)

type ReplicaSet struct {
	replicas map[types.ReplicaID]bool
	size     int
	mtx      *sync.Mutex
}

func NewReplicaSet() *ReplicaSet {
	return &ReplicaSet{
		replicas: make(map[types.ReplicaID]bool),
		size:     0,
		mtx:      new(sync.Mutex),
	}
}

func (r *ReplicaSet) Add(id types.ReplicaID) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	_, ok := r.replicas[id]
	if !ok {
		r.replicas[id] = true
		r.size = r.size + 1
	}
}

func (r *ReplicaSet) Size() int {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return r.size
}

func (r *ReplicaSet) Exists(id types.ReplicaID) bool {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	_, ok := r.replicas[id]
	return ok
}

func (r *ReplicaSet) Iter() []types.ReplicaID {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	result := make([]types.ReplicaID, len(r.replicas))
	i := 0
	for r := range r.replicas {
		result[i] = r
		i = i + 1
	}
	return result
}

// Should cache key instead of decoding everytime
func GetPrivKey(r *types.Replica) (crypto.PrivKey, error) {
	privKey, ok := r.Info["private_key"]
	if !ok {
		return nil, errors.New("private key information does not exist")
	}
	privKeyMap, ok := privKey.(map[string]interface{})
	if !ok {
		return nil, errors.New("key information invalid")
	}
	keyType := privKeyMap["type"].(string)
	keyEncoding := privKeyMap["key"].(string)

	switch keyType {
	case ed25519.KeyType:
		key, err := base64.StdEncoding.DecodeString(keyEncoding)
		if err != nil {
			return nil, errors.New("malformed private key")
		}
		return ed25519.PrivKey(key), nil
	case secp256k1.KeyType:
		key, err := base64.StdEncoding.DecodeString(keyEncoding)
		if err != nil {
			return nil, errors.New("malformed private key")
		}
		return secp256k1.PrivKey(key), nil
	default:
		return nil, errors.New("key type invalid")
	}
}

func GetChainID(r *types.Replica) (string, error) {
	chain_id, ok := r.Info["chain_id"]
	if !ok {
		return "", errors.New("chain id does not exist")
	}
	return chain_id.(string), nil
}
