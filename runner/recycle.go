package runner

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	gErrors "github.com/cloudbase/garm-provider-common/errors"
	commonParams "github.com/cloudbase/garm-provider-common/params"

	"github.com/cloudbase/garm/params"
	"github.com/cloudbase/garm/runner/common"
)

const maxRecycleTasks = 20

var (
	recycleTasks     = map[string]*params.RecycleTask{}
	recycleTasksLock sync.Mutex
)

func saveRecycleTask(task params.RecycleTask) {
	recycleTasksLock.Lock()
	defer recycleTasksLock.Unlock()

	recycleTasks[task.ID] = &task

	if len(recycleTasks) <= maxRecycleTasks {
		return
	}

	var oldestID string
	var oldest time.Time
	for id, candidate := range recycleTasks {
		if oldestID == "" || candidate.StartedAt.Before(oldest) {
			oldestID = id
			oldest = candidate.StartedAt
		}
	}
	delete(recycleTasks, oldestID)
}

func updateRecycleTask(id string, update func(*params.RecycleTask)) {
	recycleTasksLock.Lock()
	defer recycleTasksLock.Unlock()

	task, ok := recycleTasks[id]
	if !ok {
		return
	}
	update(task)
	task.UpdatedAt = time.Now().UTC()
}

// GetRecycleTask returns the state of a previously started replacement task.
func (r *Runner) GetRecycleTask(ctx context.Context, taskID string) (params.RecycleTask, error) {
	recycleTasksLock.Lock()
	defer recycleTasksLock.Unlock()

	task, ok := recycleTasks[taskID]
	if !ok {
		return params.RecycleTask{}, gErrors.NewNotFoundError("recycle task %s", taskID)
	}

	ret := *task
	ret.Failures = append([]params.RecycleFailure{}, task.Failures...)
	return ret, nil
}

// filterRecycleTargets keeps the instances selected by a replacement request.
func filterRecycleTargets(instances []params.Instance, onlyIdle bool, idFilter map[string]struct{}) []params.Instance {
	targets := []params.Instance{}

	for _, inst := range instances {
		if inst.Status != commonParams.InstanceRunning {
			continue
		}
		if len(idFilter) > 0 {
			if _, ok := idFilter[inst.ID]; !ok {
				continue
			}
		}
		if onlyIdle && inst.RunnerStatus != params.RunnerIdle {
			continue
		}
		targets = append(targets, inst)
	}

	return targets
}

// RecyclePoolInstances deletes the selected pool instances. The pool recreates
// them with its current image and template.
func (r *Runner) RecyclePoolInstances(ctx context.Context, poolID string, param params.RecyclePoolInstancesParams) (params.RecycleTask, error) {
	pool, err := r.store.GetPoolByID(ctx, poolID)
	if err != nil {
		return params.RecycleTask{}, fmt.Errorf("error fetching pool: %w", err)
	}

	if !pool.Enabled {
		return params.RecycleTask{}, gErrors.NewConflictError("pool %s is disabled", poolID)
	}

	minIdle := pool.MinIdleRunnersAsInt()
	if minIdle == 0 {
		return params.RecycleTask{}, gErrors.NewBadRequestError(
			"pool %s does not keep idle runners, replaced instances are not recreated", poolID)
	}

	instances, err := r.store.ListPoolInstances(ctx, poolID, false)
	if err != nil {
		return params.RecycleTask{}, fmt.Errorf("error listing pool instances: %w", err)
	}

	onlyIdle := true
	if param.OnlyIdle != nil {
		onlyIdle = *param.OnlyIdle
	}

	idFilter := map[string]struct{}{}
	for _, id := range param.InstanceIDs {
		idFilter[id] = struct{}{}
	}

	targets := filterRecycleTargets(instances, onlyIdle, idFilter)
	if len(targets) == 0 {
		return params.RecycleTask{}, gErrors.NewBadRequestError("no instances match the request")
	}

	concurrency := param.Concurrency
	if concurrency == 0 {
		concurrency = 1
	}
	if concurrency > uint(len(targets)) {
		concurrency = uint(len(targets))
	}

	timeout := time.Duration(pool.RunnerBootstrapTimeout) * time.Minute
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}

	now := time.Now().UTC()
	task := params.RecycleTask{
		ID:        uuid.New().String(),
		PoolID:    poolID,
		State:     params.RecycleTaskPending,
		Total:     uint(len(targets)),
		StartedAt: now,
		UpdatedAt: now,
	}
	saveRecycleTask(task)

	// The request context ends when the handler returns.
	go r.runRecycleTask(context.WithoutCancel(ctx), pool, targets, minIdle, concurrency, timeout, task.ID)

	return task, nil
}

func (r *Runner) runRecycleTask(ctx context.Context, pool params.Pool, targets []params.Instance, minIdle int, concurrency uint, timeout time.Duration, taskID string) {
	updateRecycleTask(taskID, func(task *params.RecycleTask) {
		task.State = params.RecycleTaskRunning
	})

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var lock sync.Mutex

	for _, inst := range targets {
		sem <- struct{}{}
		wg.Add(1)
		go func(inst params.Instance) {
			defer wg.Done()
			defer func() { <-sem }()

			err := r.recycleOneInstance(ctx, pool.ID, inst, minIdle, timeout)

			lock.Lock()
			defer lock.Unlock()

			if err != nil {
				slog.With(slog.Any("error", err)).ErrorContext(
					ctx, "failed to replace instance", "instance", inst.Name)
				updateRecycleTask(taskID, func(task *params.RecycleTask) {
					task.Failed++
					task.Failures = append(task.Failures, params.RecycleFailure{
						Instance: inst.Name,
						Error:    err.Error(),
					})
				})
				return
			}

			updateRecycleTask(taskID, func(task *params.RecycleTask) {
				task.Replaced++
			})
		}(inst)
	}

	wg.Wait()

	updateRecycleTask(taskID, func(task *params.RecycleTask) {
		if task.Failed > 0 {
			task.State = params.RecycleTaskFailed
			return
		}
		task.State = params.RecycleTaskCompleted
	})
}

func (r *Runner) recycleOneInstance(ctx context.Context, poolID string, inst params.Instance, minIdle int, timeout time.Duration) error {
	if err := r.deleteRunnerInternal(ctx, inst.Name, false, false); err != nil {
		return err
	}

	return r.waitForReplacement(ctx, poolID, inst.Name, minIdle, timeout)
}

// waitForReplacement waits until the deleted instance is gone and the pool has
// at least minIdle idle instances again.
func (r *Runner) waitForReplacement(ctx context.Context, poolID, instanceName string, minIdle int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for {
		instances, err := r.store.ListPoolInstances(ctx, poolID, false)

		removed := true
		idle := 0
		if err == nil {
			for _, inst := range instances {
				if inst.Name == instanceName {
					removed = false
				}
				if inst.Status == commonParams.InstanceRunning && inst.RunnerStatus == params.RunnerIdle {
					idle++
				}
			}
		}

		if removed && idle >= minIdle {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for instance %s to be replaced", instanceName)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(common.PoolConsilitationInterval):
		}
	}
}
