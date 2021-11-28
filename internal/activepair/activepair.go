package activepair

import (
	"sync"

	"github.com/georlav/bitstamp"
)

type ActivePair struct {
	p  bitstamp.Pair
	mu sync.RWMutex
}

func NewActivePair(p bitstamp.Pair) ActivePair {
	return ActivePair{
		p: p,
	}
}

func (a *ActivePair) Set(p bitstamp.Pair) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.p = p
}

func (a *ActivePair) Get() bitstamp.Pair {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.p
}
