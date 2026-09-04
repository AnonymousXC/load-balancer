package balancer

import (
	"hash/fnv"
	"sort"
	"sync"
)

type ConsistentHash struct {
	virtualNodes int
	hashRing     []uint32
	nodeMap      map[uint32]*Backend
	mu           sync.RWMutex
}

func NewConsistentHash(virtualNodes int) *ConsistentHash {
	if virtualNodes <= 0 {
		virtualNodes = 150
	}
	return &ConsistentHash{
		virtualNodes: virtualNodes,
		nodeMap:      make(map[uint32]*Backend),
	}
}

func (ch *ConsistentHash) Select(pool *Pool) *Backend {
	nodes := pool.HealthyBackends()
	if len(nodes) == 0 {
		return nil
	}

	ch.mu.RLock()
	if len(ch.hashRing) == 0 {
		ch.mu.RUnlock()
		ch.buildHashRing(nodes)
		ch.mu.RLock()
	}
	defer ch.mu.RUnlock()

	return nodes[0]
}

func (ch *ConsistentHash) buildHashRing(nodes []*Backend) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	ch.hashRing = make([]uint32, 0)
	ch.nodeMap = make(map[uint32]*Backend)

	for _, node := range nodes {
		for i := 0; i < ch.virtualNodes; i++ {
			key := ch.hashKey(node.URL.String(), i)
			ch.hashRing = append(ch.hashRing, key)
			ch.nodeMap[key] = node
		}
	}

	sort.Slice(ch.hashRing, func(i, j int) bool {
		return ch.hashRing[i] < ch.hashRing[j]
	})
}

func (ch *ConsistentHash) hashKey(node string, index int) uint32 {
	h := fnv.New32a()
	h.Write([]byte(node))
	h.Write([]byte(string(rune(index))))
	return h.Sum32()
}

func (ch *ConsistentHash) GetNode(hash uint32) *Backend {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if len(ch.hashRing) == 0 {
		return nil
	}

	idx := sort.Search(len(ch.hashRing), func(i int) bool {
		return ch.hashRing[i] >= hash
	})

	if idx == len(ch.hashRing) {
		idx = 0
	}

	return ch.nodeMap[ch.hashRing[idx]]
}
