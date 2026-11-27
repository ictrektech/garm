package main

import (
	"strings"
	"testing"
)

func TestImageProblem(t *testing.T) {
	valid := []string{
		"registry.example.com/ci/actions-runner:jetson-thor-1",
		"localhost/runner:latest",
		"registry.example.com:5000/ci/runner:v1.2.3",
		"ghcr.io/actions/actions-runner:latest",
		"registry.example.com/ci/runner@sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
	for _, image := range valid {
		if problem := imageProblem(image); problem != "" {
			t.Errorf("%s: unexpected problem %q", image, problem)
		}
	}

	invalid := map[string]string{
		"":                                 "empty",
		"actions-runner":                   "registry",
		"garm-runner:1":                    "registry",
		"ghcr.io/actions/actions-runner":   "tag",
		"registry.example.com/ci/Runner:1": "lowercase",
		"Registry.example.com/ci/runner:1": "lowercase",
		"registry.example.com/ci/runner@sha256:ab": "digest",
	}
	for image, want := range invalid {
		problem := imageProblem(image)
		if problem == "" {
			t.Errorf("%s: expected a problem mentioning %q", image, want)
		}
	}
}

func TestGrantProblemNamesTheMissingGrant(t *testing.T) {
	req := createRequest{
		Target: target{Kind: "ssh", ID: "01M83P7A", Profile: "jetson-orin"},
		Image:  "swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm-runner:arm_v6.0.1_27ec9940",
	}
	got := grantProblem(req)
	for _, want := range []string{"01M83P7A", "jetson-orin", "swr.cn-southwest-2.myhuaweicloud.com/ictrek/garm-runner"} {
		if !strings.Contains(got, want) {
			t.Errorf("message does not mention %q: %s", want, got)
		}
	}
	if strings.Contains(got, "27ec9940") {
		t.Errorf("repository still carries the tag: %s", got)
	}
}

func TestGrantProblemStripsDigest(t *testing.T) {
	req := createRequest{
		Image: "registry.example.com/ci/runner@sha256:" + strings.Repeat("a", 64),
	}
	if got := grantProblem(req); strings.Contains(got, "sha256") {
		t.Errorf("repository still carries the digest: %s", got)
	}
}

func TestExplainRejectionForForbiddenPointsAtTheGrant(t *testing.T) {
	req := createRequest{
		RequestKey:    "garm-abc",
		Target:        target{Kind: "ssh", ID: "node-1", Profile: "jetson-thor"},
		Image:         "registry.example.com/ci/runner:1",
		ContainerPort: 8080,
	}
	err := explainRejection(&httpError{method: "POST", path: "/backend-deployments", status: 403, body: "forbidden"}, req)
	if !strings.Contains(err.Error(), "no local rule matched") || !strings.Contains(err.Error(), "node-1") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRequestProblems(t *testing.T) {
	good := createRequest{
		RequestKey:    "garm-abc123",
		Target:        target{Kind: "ssh", ID: "node-35", Profile: "jetson-thor"},
		Image:         "registry.example.com/ci/runner:1",
		ContainerPort: 8080,
		Config:        map[string]any{"pool_id": "pool-1"},
	}
	if problems := requestProblems(good); len(problems) != 0 {
		t.Fatalf("valid request rejected: %v", problems)
	}

	bad := good
	bad.Target.Kind = "vm"
	cpu := uint32(50)
	bad.CPUMillicores = &cpu
	bad.Image = "runner:1"
	problems := requestProblems(bad)
	if len(problems) != 3 {
		t.Fatalf("want 3 problems (kind, cpu, image), got %d: %v", len(problems), problems)
	}
}
