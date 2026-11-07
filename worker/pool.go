package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"optionsbacktest/engine"
	"optionsbacktest/models"
	"optionsbacktest/repository"
	"optionsbacktest/scripting"
	"optionsbacktest/utils"
)

const queueCapacity = 100

type WorkerPool struct {
	jobs      chan int64
	startOnce sync.Once
	repo      *repository.Repository
	logger    *slog.Logger
}

func NewWorkerPool(repo *repository.Repository, logger *slog.Logger) *WorkerPool {
	return &WorkerPool{
		jobs:   make(chan int64, queueCapacity),
		repo:   repo,
		logger: logger.With("component", "backtest_worker"),
	}
}

func (p *WorkerPool) Start() {
	p.startOnce.Do(func() {
		go func() {
			for job := range p.jobs {
				p.processJob(job)
			}
		}()
	})
}

func (p *WorkerPool) processJob(id int64) {
	// A queued job outlives the HTTP request that submitted it.
	ctx := context.Background()
	backtest, err := p.repo.GetBacktest(ctx, id)
	if err != nil {
		p.logger.ErrorContext(ctx, "failed to fetch backtest", "backtest_id", id, "error", err)
		return
	}
	script, err := scripting.LoadSource(fmt.Sprintf("backtest_%d.star", id), backtest.Script)
	var result *engine.BacktestResult
	if err == nil {
		result, err = script.Run(ctx, engine.NewEngine(p.repo))
	}
	var payload []byte
	if err == nil {
		payload, err = utils.MarshalBacktestResult(result)
	}
	status := models.BacktestCompleted
	if err != nil {
		status = models.BacktestFailed
		payload, _ = json.Marshal(map[string]string{"error": err.Error()})
		p.logger.ErrorContext(ctx, "backtest failed", "backtest_id", id, "error", err)
	}
	if err := p.repo.UpdateBacktest(ctx, id, status, payload); err != nil {
		p.logger.ErrorContext(ctx, "failed to persist backtest result", "backtest_id", id, "error", err)
	}
}

func (p *WorkerPool) SubmitJob(id int64) {
	p.jobs <- id
}
