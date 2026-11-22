package evaluation

import (
	"context"
	"sync"
)

type Result struct {
	Sample  Sample  `json:"sample"`
	Outcome Outcome `json:"outcome"`
}

// Run executes classification for all samples with the specified concurrency.
func Run(
	ctx context.Context,
	samples []Sample,
	call func(context.Context, Sample) Outcome,
	concurrency int,
) []Result {
	if concurrency < 1 {
		concurrency = 1
	}

	results := make([]Result, len(samples))
	if len(samples) == 0 {
		return results
	}

	if concurrency == 1 {
		for i, s := range samples {
			results[i] = Result{
				Sample:  s,
				Outcome: call(ctx, s),
			}
		}

		return results
	}

	jobs := make(chan int, len(samples))
	for i := range samples {
		jobs <- i
	}
	close(jobs)

	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for i := range jobs {
				if err := ctx.Err(); err != nil {
					results[i] = Result{
						Sample: samples[i],
						Outcome: Outcome{
							Err: err,
						},
					}

					continue
				}

				results[i] = Result{
					Sample:  samples[i],
					Outcome: call(ctx, samples[i]),
				}
			}
		}()
	}

	wg.Wait()

	return results
}
