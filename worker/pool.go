package worker

import "sync"

const queueCapacity = 100

type WorkerPool struct {
	jobs      chan int64
	startOnce sync.Once
}

func NewWorkerPool() *WorkerPool {
	return &WorkerPool{jobs: make(chan int64, queueCapacity)}
}

func (p *WorkerPool) Start() {
	p.startOnce.Do(func() {
		go func() {
			for job := range p.jobs {
				_ = job
				// TODO: Process the backtest identified by job.
			}
		}()
	})
}

func (p *WorkerPool) SubmitJob(id int64) {
	p.jobs <- id
}
