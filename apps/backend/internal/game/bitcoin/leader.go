package bitcoin

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	leaderKey     = "bitcoin:price_leader"
	leaderTTL     = 15 * time.Second
	renewInterval = 5 * time.Second
)

type PriceLeader struct {
	rdb       *redis.Client
	replicaID string
	isLeader  bool
	mu        sync.RWMutex
	done      chan struct{}
	once      sync.Once
}

func NewPriceLeader(rdb *redis.Client, replicaID string) *PriceLeader {
	return &PriceLeader{
		rdb:       rdb,
		replicaID: replicaID,
		done:      make(chan struct{}),
	}
}

func (l *PriceLeader) Start(ctx context.Context) {
	l.tryAcquireOrRenew(ctx)
	go l.electionLoop(ctx)
}

func (l *PriceLeader) electionLoop(ctx context.Context) {
	ticker := time.NewTicker(renewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			l.tryAcquireOrRenew(ctx)
		case <-l.done:
			if l.IsLeader() {
				l.rdb.Del(ctx, leaderKey)
				log.Printf("[bitcoin-leader] released leadership: %s", l.replicaID)
			}
			return
		case <-ctx.Done():
			return
		}
	}
}

func (l *PriceLeader) tryAcquireOrRenew(ctx context.Context) {
	ok, err := l.rdb.SetNX(ctx, leaderKey, l.replicaID, leaderTTL).Result()
	if err != nil {
		return
	}
	if ok {
		l.mu.Lock()
		if !l.isLeader {
			log.Printf("[bitcoin-leader] acquired leadership: %s", l.replicaID)
		}
		l.isLeader = true
		l.mu.Unlock()
		return
	}
	holder, err := l.rdb.Get(ctx, leaderKey).Result()
	if err != nil {
		return
	}
	if holder == l.replicaID {
		l.rdb.Expire(ctx, leaderKey, leaderTTL)
		l.mu.Lock()
		l.isLeader = true
		l.mu.Unlock()
	} else {
		l.mu.Lock()
		if l.isLeader {
			log.Printf("[bitcoin-leader] lost leadership: %s", l.replicaID)
		}
		l.isLeader = false
		l.mu.Unlock()
	}
}

func (l *PriceLeader) IsLeader() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.isLeader
}

func (l *PriceLeader) Stop() {
	l.once.Do(func() { close(l.done) })
}
