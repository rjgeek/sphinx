package worker

import (
	"github.com/shx-project/sphinx/blockchain"
	"sync"
	"sync/atomic"
	"time"
)

type WorkPending struct {
	errored	int32
	Pending 	[]*Work
	rwlock		sync.RWMutex
	//InputCh		chan *Work
}

func NewWorkPending() *WorkPending{
	return &WorkPending{errored: 0, Pending:make([]*Work, 0)}
}

func (p *WorkPending)HaveErr() bool {
	return atomic.LoadInt32(&p.errored) > 0
}

func (p *WorkPending)Add(w *Work) bool {
	p.rwlock.Lock()
	defer p.rwlock.Unlock()

	if p.HaveErr() {
		return false
	}
	p.Pending = append(p.Pending, w)
	return true
}

func (p *WorkPending)Top() *Work {
	p.rwlock.RLock()
	defer p.rwlock.RUnlock()

	if !p.HaveErr() && len(p.Pending) > 0 {
		return p.Pending[len(p.Pending)-1]
	}
	return nil
}

func (p *WorkPending)Head() *Work {
	p.rwlock.RLock()
	defer p.rwlock.RUnlock()

	if !p.HaveErr() && len(p.Pending) > 0 {
		return p.Pending[0]
	}
	return nil
}

func (p *WorkPending) Empty() bool {
	p.rwlock.RLock()
	defer p.rwlock.RUnlock()

	return len(p.Pending) == 0
}

func (p *WorkPending)SetNoError() {
	atomic.StoreInt32(&p.errored, 0)
}

func (p *WorkPending)pop() *Work {
	p.rwlock.Lock()
	defer p.rwlock.Unlock()
	if len(p.Pending) > 0 {
		w := p.Pending[0]
		p.Pending = p.Pending[1:]
		return w
	}
	return nil
}

func (p *WorkPending) Run() {
	chain := bc.InstanceBlockChain()
	duration := time.Millisecond * 500
	timer := time.NewTimer(duration)
	defer timer.Stop()
	for true {
		select {
		case <- timer.C:
			start := time.Now().Unix()
			if p.HaveErr() {
				for len(p.Pending) > 0 {
					w := p.pop()
					w.WorkEnded(false)
				}
			}
			for !p.HaveErr() && len(p.Pending) > 0 {

				now := time.Now().Unix()
				if now - start > 2 {
					break
				}
				w := p.Head()
				_, err := chain.WriteBlockAndState(w.Block, w.receipts, w.state)
				if err != nil {
					// enter err mode, not work and receive new work.
					atomic.StoreInt32(&p.errored,1)
					w.WorkEnded(false)
				} else {
					w.WorkEnded(true)
				}
				p.pop()
			}
			timer.Reset(duration)
		}
	}
}
