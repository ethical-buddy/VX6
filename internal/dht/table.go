package dht

import (
	"crypto/sha256"
	"math/big"
	"sort"
	"sync"

	"github.com/vx6/vx6/internal/proto"
)

const K = 20 // K-bucket size

type RoutingTable struct {
	SelfID string
	mu     sync.RWMutex
	Buckets [256][]proto.NodeInfo // Full bit length of SHA-256
}

func NewRoutingTable(selfID string) *RoutingTable {
	return &RoutingTable{SelfID: selfID}
}

// AddNode inserts or updates a node in the routing table
func (rt *RoutingTable) AddNode(node proto.NodeInfo) {
	if node.ID == rt.SelfID {
		return
	}

	dist := rt.distance(rt.SelfID, node.ID)
	bucketIdx := dist.BitLen() - 1
	if bucketIdx < 0 {
		bucketIdx = 0
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	bucket := rt.Buckets[bucketIdx]
	for i, existing := range bucket {
		if existing.ID == node.ID {
			// Update existing entry (move to end)
			rt.Buckets[bucketIdx] = append(bucket[:i], bucket[i+1:]...)
			rt.Buckets[bucketIdx] = append(rt.Buckets[bucketIdx], node)
			return
		}
	}

	if len(bucket) < K {
		rt.Buckets[bucketIdx] = append(bucket, node)
	}
}

// ClosestNodes returns the K closest nodes to the target ID
func (rt *RoutingTable) ClosestNodes(targetID string, count int) []proto.NodeInfo {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	var all []proto.NodeInfo
	for _, b := range rt.Buckets {
		all = append(all, b...)
	}

	sort.Slice(all, func(i, j int) bool {
		distI := rt.distance(all[i].ID, targetID)
		distJ := rt.distance(all[j].ID, targetID)
		return distI.Cmp(distJ) == -1
	})

	if len(all) > count {
		return all[:count]
	}
	return all
}

func (rt *RoutingTable) distance(id1, id2 string) *big.Int {
	h1 := sha256.Sum256([]byte(id1))
	h2 := sha256.Sum256([]byte(id2))

	i1 := new(big.Int).SetBytes(h1[:])
	i2 := new(big.Int).SetBytes(h2[:])

	return new(big.Int).Xor(i1, i2)
}
