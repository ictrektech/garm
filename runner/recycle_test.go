package runner

import (
	"testing"

	commonParams "github.com/cloudbase/garm-provider-common/params"

	"github.com/cloudbase/garm/params"
)

func instanceNames(instances []params.Instance) []string {
	ret := make([]string, 0, len(instances))
	for _, inst := range instances {
		ret = append(ret, inst.Name)
	}
	return ret
}

func TestFilterRecycleTargets(t *testing.T) {
	idle := params.Instance{
		ID: "1", Name: "idle",
		Status: commonParams.InstanceRunning, RunnerStatus: params.RunnerIdle,
	}
	active := params.Instance{
		ID: "2", Name: "active",
		Status: commonParams.InstanceRunning, RunnerStatus: params.RunnerActive,
	}
	dying := params.Instance{
		ID: "3", Name: "dying",
		Status: commonParams.InstancePendingDelete, RunnerStatus: params.RunnerIdle,
	}
	instances := []params.Instance{idle, active, dying}

	t.Run("idle only skips active and dying instances", func(t *testing.T) {
		got := filterRecycleTargets(instances, true, nil)
		if len(got) != 1 || got[0].Name != "idle" {
			t.Fatalf("got %v, want [idle]", instanceNames(got))
		}
	})

	t.Run("non idle skips dying instances", func(t *testing.T) {
		got := filterRecycleTargets(instances, false, nil)
		if len(got) != 2 || got[0].Name != "idle" || got[1].Name != "active" {
			t.Fatalf("got %v, want [idle active]", instanceNames(got))
		}
	})

	t.Run("id filter selects one instance", func(t *testing.T) {
		got := filterRecycleTargets(instances, false, map[string]struct{}{"2": {}})
		if len(got) != 1 || got[0].Name != "active" {
			t.Fatalf("got %v, want [active]", instanceNames(got))
		}
	})
}
