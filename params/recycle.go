package params

import "time"

// RecycleTaskState is the state of a pool instance replacement task.
type RecycleTaskState string

const (
	RecycleTaskPending   RecycleTaskState = "pending"
	RecycleTaskRunning   RecycleTaskState = "running"
	RecycleTaskCompleted RecycleTaskState = "completed"
	RecycleTaskFailed    RecycleTaskState = "failed"
)

// RecyclePoolInstancesParams holds the options of a pool instance
// replacement request.
type RecyclePoolInstancesParams struct {
	// Concurrency is the number of instances replaced at the same time.
	Concurrency uint `json:"concurrency,omitempty"`

	// OnlyIdle restricts the replacement to idle instances. Defaults to true.
	OnlyIdle *bool `json:"only_idle,omitempty"`

	// InstanceIDs limits the replacement to the listed instances.
	InstanceIDs []string `json:"instance_ids,omitempty"`
}

// RecycleFailure describes why one instance could not be replaced.
type RecycleFailure struct {
	Instance string `json:"instance"`
	Error    string `json:"error"`
}

// RecycleTask reports the progress of a pool instance replacement.
type RecycleTask struct {
	ID        string           `json:"id"`
	PoolID    string           `json:"pool_id"`
	State     RecycleTaskState `json:"state"`
	Total     uint             `json:"total"`
	Replaced  uint             `json:"replaced"`
	Failed    uint             `json:"failed"`
	Failures  []RecycleFailure `json:"failures,omitempty"`
	StartedAt time.Time        `json:"started_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}
