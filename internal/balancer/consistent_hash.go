package balancer

import (
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
	"sync/atomic"
)

type KeyedStrategy interface {
	Strategy
	SelectKey(pool *Pool, key string) *Backend
}

type ConsistentHash struct {
	virtualNodes  int
	hashRing      []uint32
	nodeMap       map[uint32]*Backend
	mu            sync.RWMutex
	lastNodesHash uint64
	counter       atomic.Uint64
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

	ch.ensureRing(nodes)

	cnt := ch.counter.Add(1)
	key := fmt.Sprintf("req-%d", cnt)
	h := ch.hashString(key)
	return ch.GetNode(h)
}

func (ch *ConsistentHash) SelectKey(pool *Pool, key string) *Backend {
	nodes := pool.HealthyBackends()
	if len(nodes) == 0 {
		return nil
	}

	ch.ensureRing(nodes)

	if key == "" {
		return ch.Select(pool)
	}

	h := ch.hashString(key)
	return ch.GetNode(h)
}

func (ch *ConsistentHash) ensureRing(nodes []*Backend) {
	var fp uint64
	for _, n := range nodes {
		for _, b := range []byte(n.URL.String()) {
			fp = fp*31 + uint64(b)
		}
	}

	ch.mu.RLock()
	if ch.lastNodesHash == fp && len(ch.hashRing) > 0 {
		ch.mu.RUnlock()
		return
	}
	ch.mu.RUnlock()

	ch.buildHashRing(nodes, fp)
}

func (ch *ConsistentHash) buildHashRing(nodes []*Backend, fp uint64) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	ch.hashRing = make([]uint32, 0, len(nodes)*ch.virtualNodes)
	ch.nodeMap = make(map[uint32]*Backend, len(nodes)*ch.virtualNodes)

	for _, node := range nodes {
		for i := 0; i < ch.virtualNodes; i++ {
			key := fmt.Sprintf("%s#%d", node.URL.String(), i)
			h := ch.hashString(key)
			ch.hashRing = append(ch.hashRing, h)
			ch.nodeMap[h] = node
		}
	}

	sort.Slice(ch.hashRing, func(i, j int) bool {
		return ch.hashRing[i] < ch.hashRing[j]
	})

	ch.lastNodesHash = fp
}

func (ch *ConsistentHash) hashString(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
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
