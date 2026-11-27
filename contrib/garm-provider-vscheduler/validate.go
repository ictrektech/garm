package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// explainRejection turns a V-Scheduler rejection into an actionable message.
func explainRejection(err error, req createRequest) error {
	var httpErr *httpError
	if !errors.As(err, &httpErr) {
		return fmt.Errorf("creating backend deployment: %w", err)
	}

	safe := req
	safe.Config = map[string]any{}
	for key, value := range req.Config {
		if key == "instance_token" {
			safe.Config[key] = "<redacted>"
			continue
		}
		safe.Config[key] = value
	}
	body, _ := json.Marshal(safe)

	problems := requestProblems(req)
	if len(problems) == 0 {
		problems = []string{grantProblem(req)}
	}
	return fmt.Errorf("V-Scheduler rejected the deployment (%d): %s; request: %s",
		httpErr.status, strings.Join(problems, "; "), body)
}

// grantProblem explains a 401/403 answer: the app has no deployment grant for
// this target or repository. V-Scheduler only says "no local rule matched", so
// print both values the operator has to add.
func grantProblem(req createRequest) string {
	repository := req.Image
	if before, _, found := strings.Cut(repository, "@"); found {
		repository = before
	}
	if idx := strings.LastIndex(repository, ":"); idx > strings.LastIndex(repository, "/") {
		repository = repository[:idx]
	}
	return fmt.Sprintf(
		"no local rule matched: grant target {kind: %q, id: %q, profile: %q} and repository %q to this app in V-Scheduler",
		req.Target.Kind, req.Target.ID, req.Target.Profile, repository)
}

// requestProblems lists the format rules V-Scheduler enforces before it accepts a
// deployment. V-Scheduler answers a bare 422 INVALID_REQUEST with no field name,
// so the same rules are checked here to make the error actionable.
//
// shortcut: mirrors backend/platform/shared/application_backend.rs and
// backend_deployments.rs; revisit when V-Scheduler changes those rules.
func requestProblems(req createRequest) []string {
	var problems []string

	if !identifier(req.RequestKey) {
		problems = append(problems, fmt.Sprintf(
			"request_key %q: must be 1-96 chars of A-Za-z0-9_.-, first char alnum", req.RequestKey))
	}
	if req.Target.Kind != "ssh" && req.Target.Kind != "cluster" {
		problems = append(problems, fmt.Sprintf("target.kind %q: must be ssh or cluster", req.Target.Kind))
	}
	if !identifier(req.Target.ID) {
		problems = append(problems, fmt.Sprintf("target.id %q: must be 1-96 chars of A-Za-z0-9_.-", req.Target.ID))
	}
	if !identifier(req.Target.Profile) {
		problems = append(problems, fmt.Sprintf("target.profile %q: must be 1-96 chars of A-Za-z0-9_.-", req.Target.Profile))
	}
	if req.ContainerPort == 0 {
		problems = append(problems, "container_port must be 1-65535")
	}
	if req.CPUMillicores != nil && (*req.CPUMillicores < 100 || *req.CPUMillicores > 128000) {
		problems = append(problems, fmt.Sprintf("cpu_millicores %d: must be 100-128000", *req.CPUMillicores))
	}
	if req.MemoryMib != nil && (*req.MemoryMib < 16 || *req.MemoryMib > 1048576) {
		problems = append(problems, fmt.Sprintf("memory_mib %d: must be 16-1048576", *req.MemoryMib))
	}
	if problem := imageProblem(req.Image); problem != "" {
		problems = append(problems, problem)
	}
	if raw, err := json.Marshal(req.Config); err == nil && len(raw) > 8192 {
		problems = append(problems, fmt.Sprintf("config is %d bytes: limit 8192", len(raw)))
	}
	return problems
}

// imageProblem checks the image reference against V-Scheduler's rules: a
// lowercase registry containing a dot or port, lowercase path segments, and a
// tag or sha256 digest.
func imageProblem(image string) string {
	if image == "" {
		return "image is empty: set the pool image to the runner container image"
	}
	if len(image) > 512 {
		return fmt.Sprintf("image %q: longer than 512 chars", image)
	}

	name := image
	hasDigest := false
	if before, after, found := strings.Cut(image, "@"); found {
		hasDigest = true
		name = before
		digest, ok := strings.CutPrefix(after, "sha256:")
		if !ok || len(digest) != 64 || !isLowerHex(digest) {
			return fmt.Sprintf("image %q: digest must be sha256:<64 lowercase hex chars>", image)
		}
	}

	hasTag := false
	if idx := strings.LastIndex(name, ":"); idx > strings.LastIndex(name, "/") {
		tag := name[idx+1:]
		hasTag = true
		name = name[:idx]
		if tag == "" || len(tag) > 128 || !isAlnum(tag[0]) && !strings.HasPrefix(tag, "_") {
			return fmt.Sprintf("image %q: tag %q is not valid", image, tag)
		}
		for i := 0; i < len(tag); i++ {
			if !isAlnum(tag[i]) && !strings.ContainsRune("._-", rune(tag[i])) {
				return fmt.Sprintf("image %q: tag %q has a character outside A-Za-z0-9._-", image, tag)
			}
		}
	}
	if !hasTag && !hasDigest {
		return fmt.Sprintf("image %q: needs a tag or a @sha256: digest", image)
	}

	registry, path, found := strings.Cut(name, "/")
	if !found {
		return fmt.Sprintf("image %q: must start with a registry, e.g. registry.example.com/ci/runner:1", image)
	}
	if registry != "localhost" && !strings.ContainsAny(registry, ".:") {
		return fmt.Sprintf("image %q: registry %q needs a dot or a port", image, registry)
	}
	for i := 0; i < len(registry); i++ {
		if b := registry[i]; !isLowerAlnum(b) && !strings.ContainsRune(".:-", rune(b)) {
			return fmt.Sprintf("image %q: registry %q must be lowercase alnum with . : -", image, registry)
		}
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || !isLowerAlnum(part[0]) || !isAlnum(part[len(part)-1]) {
			return fmt.Sprintf("image %q: path segment %q must start and end alnum", image, part)
		}
		for i := 0; i < len(part); i++ {
			if b := part[i]; !isLowerAlnum(b) && !strings.ContainsRune("._-", rune(b)) {
				return fmt.Sprintf("image %q: path segment %q must be lowercase alnum with . _ -", image, part)
			}
		}
	}
	return ""
}

func identifier(value string) bool {
	if len(value) < 1 || len(value) > 96 || !isAlnum(value[0]) {
		return false
	}
	for i := 0; i < len(value); i++ {
		if b := value[i]; !isAlnum(b) && !strings.ContainsRune("_.-", rune(b)) {
			return false
		}
	}
	return true
}

func isAlnum(b byte) bool {
	return isLowerAlnum(b) || b >= 'A' && b <= 'Z'
}

func isLowerAlnum(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9'
}

func isLowerHex(value string) bool {
	for i := 0; i < len(value); i++ {
		b := value[i]
		if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
			return false
		}
	}
	return true
}
