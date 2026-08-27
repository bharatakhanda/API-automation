package runner

import (
	"context"
	"sync"

	"api-automation/internal/engine"
	"api-automation/internal/model"
)

type Options struct {
	Concurrency int
	StopOnError bool
}

type Runner struct {
	executor *engine.Executor
}

func New(executor *engine.Executor) *Runner { return &Runner{executor: executor} }

func (r *Runner) Run(ctx context.Context, workflow model.Workflow, opts Options) <-chan model.Result {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 1
	}

	out := make(chan model.Result)
	go func() {
		defer close(out)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		jobs := make(chan model.Request)
		var wg sync.WaitGroup
		for i := 0; i < opts.Concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for req := range jobs {
					res := r.executor.Do(ctx, req)
					select {
					case out <- res:
					case <-ctx.Done():
						return
					}
					if opts.StopOnError && res.Error != "" {
						cancel()
					}
				}
			}()
		}

		for _, req := range workflow.Requests {
			select {
			case jobs <- req:
			case <-ctx.Done():
				close(jobs)
				wg.Wait()
				return
			}
		}
		close(jobs)
		wg.Wait()
	}()
	return out
}
