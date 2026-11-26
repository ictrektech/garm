package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"sort"
	"strings"
	"time"
)

// bootstrap is the subset of the GARM BootstrapInstance this provider needs.
type bootstrap struct {
	Name          string          `json:"name"`
	Image         string          `json:"image"`
	OSArch        string          `json:"arch"`
	CallbackURL   string          `json:"callback-url"`
	MetadataURL   string          `json:"metadata-url"`
	InstanceToken string          `json:"instance-token"`
	PoolID        string          `json:"pool_id"`
	RepoURL       string          `json:"repo_url"`
	Labels        []string        `json:"labels"`
	ExtraSpecs    json.RawMessage `json:"extra_specs"`
}

// poolSpecs are the pool level extra_specs. Unknown keys are ignored, so the
// same extra_specs may carry GARM's own runner install settings.
type poolSpecs struct {
	Profile       string  `json:"profile"`
	TargetKind    string  `json:"target_kind"`
	TargetID      string  `json:"target_id"`
	CPUMillicores *uint32 `json:"cpu_millicores"`
	MemoryMib     *uint32 `json:"memory_mib"`
	ContainerPort *uint16 `json:"container_port"`
	// ImageTag overrides config.image_tag for this pool.
	ImageTag string `json:"image_tag"`
}

// providerInstance is the GARM ProviderInstance this provider reports back.
type providerInstance struct {
	ProviderID string `json:"provider_id,omitempty"`
	Name       string `json:"name,omitempty"`
	Status     string `json:"status,omitempty"`
}

func emit(inst providerInstance) error {
	return json.NewEncoder(os.Stdout).Encode(inst)
}

func createInstance(api *api, cfg config, poolID string) error {
	var boot bootstrap
	if err := json.NewDecoder(os.Stdin).Decode(&boot); err != nil {
		return fmt.Errorf("decoding bootstrap params: %w", err)
	}
	if boot.Name == "" {
		return errors.New("bootstrap params have no name")
	}
	if boot.Image == "" {
		return errors.New("pool has no image set; set the pool image to the runner container image")
	}

	specs, err := specsFor(boot)
	if err != nil {
		return err
	}
	if poolID == "" {
		poolID = boot.PoolID
	}

	image, err := resolveImage(boot.Image, specs.ImageTag, cfg.ImageTag, boot.OSArch)
	if err != nil {
		return err
	}

	target, err := resolveTarget(api, specs, boot.Name, boot.OSArch)
	if err != nil {
		return err
	}

	port := cfg.ContainerPort
	if specs.ContainerPort != nil {
		port = *specs.ContainerPort
	}

	req := createRequest{
		RequestKey:    boot.Name,
		Target:        target,
		Image:         image,
		ContainerPort: port,
		CPUMillicores: specs.CPUMillicores,
		MemoryMib:     specs.MemoryMib,
		Config:        runnerConfig(boot, poolID),
	}
	dep, err := api.create(req)
	if err != nil {
		return explainRejection(err, req)
	}
	return emit(providerInstance{ProviderID: dep.ID, Name: boot.Name, Status: "running"})
}

// runnerConfig is what the runner container reads from
// V_SCHEDULER_BACKEND_CONFIG_B64. Labels are only set by GARM when it has no JIT
// config (the Gitea path), so they are empty for GitHub runners.
func runnerConfig(boot bootstrap, poolID string) map[string]any {
	return map[string]any{
		"metadata_url":   boot.MetadataURL,
		"callback_url":   boot.CallbackURL,
		"instance_token": boot.InstanceToken,
		"pool_id":        poolID,
		"runner_name":    boot.Name,
		"repo_url":       boot.RepoURL,
		"labels":         strings.Join(boot.Labels, ","),
	}
}

// resolveImage completes a pool image that carries no tag. V-Scheduler refuses
// references without a tag or digest, and the tag depends on the pool
// architecture, so users only have to write the repository.
func resolveImage(poolImage, poolTag, providerTag, osArch string) (string, error) {
	if hasTagOrDigest(poolImage) {
		return poolImage, nil
	}
	tag := poolTag
	if tag == "" {
		tag = providerTag
	}
	if tag == "" {
		return "", fmt.Errorf("pool image %q has no tag and no image_tag is configured", poolImage)
	}
	if strings.HasPrefix(tag, "amd_") || strings.HasPrefix(tag, "arm_") {
		return "", fmt.Errorf("image_tag %q: drop the architecture prefix, the provider adds it", tag)
	}
	prefix, err := archPrefix(osArch)
	if err != nil {
		return "", err
	}
	return poolImage + ":" + prefix + "_" + tag, nil
}

// archPrefix maps a pool os_arch to the image tag prefix used by
// build_runner_image.sh.
func archPrefix(osArch string) (string, error) {
	switch osArch {
	case "amd64", "x86_64", "x86-64", "x64":
		return "amd", nil
	case "arm64", "aarch64", "arm64/v8":
		return "arm", nil
	}
	return "", fmt.Errorf("pool os_arch %q: set amd64 or arm64 so the image tag can be completed", osArch)
}

// checkArch fails early when the pool os_arch and the chosen node disagree.
// V-Scheduler reports the node architecture as aarch64, x86_64 and so on.
func checkArch(poolArch, nodeArch string) error {
	if nodeArch == "" {
		return nil
	}
	poolPrefix, err := archPrefix(poolArch)
	if err != nil {
		return err
	}
	nodePrefix, err := archPrefix(nodeArch)
	if err != nil {
		// An unknown node architecture must not block placement.
		return nil
	}
	if poolPrefix != nodePrefix {
		return fmt.Errorf("pool os_arch %q does not match node arch %q", poolArch, nodeArch)
	}
	return nil
}

// hasTagOrDigest uses the same rule V-Scheduler applies: a colon after the last
// slash is a tag, an at-sign is a digest.
func hasTagOrDigest(image string) bool {
	if strings.Contains(image, "@") {
		return true
	}
	return strings.LastIndex(image, ":") > strings.LastIndex(image, "/")
}

// specsFor prefers GARM_POOL_EXTRASPECS (interface v0.1.1). Interface v0.1.0
// does not set that variable, so fall back to the extra_specs in stdin.
func specsFor(boot bootstrap) (poolSpecs, error) {
	specs, err := poolSpecsFromEnv()
	if err != nil {
		return specs, err
	}
	if specs == (poolSpecs{}) && len(boot.ExtraSpecs) > 0 {
		if err := json.Unmarshal(boot.ExtraSpecs, &specs); err != nil {
			return specs, fmt.Errorf("parsing extra_specs from bootstrap params: %w", err)
		}
	}
	return specs, nil
}

// resolveTarget picks the V-Scheduler deployment target for one runner.
func resolveTarget(api *api, specs poolSpecs, instanceName, osArch string) (target, error) {
	kind := specs.TargetKind
	if kind == "" {
		kind = "ssh"
	}

	nodes, err := api.nodes()
	if err != nil {
		return target{}, fmt.Errorf("listing managed nodes: %w", err)
	}

	if specs.TargetID != "" {
		for _, candidate := range nodes {
			if candidate.ID != specs.TargetID {
				continue
			}
			profile := specs.Profile
			if profile == "" {
				profile = candidate.Profile
			}
			if profile == "" {
				return target{}, fmt.Errorf("node %s has no profile; set extra_specs profile", candidate.ID)
			}
			if err := checkArch(osArch, candidate.Arch); err != nil {
				return target{}, err
			}
			return target{Kind: kind, ID: candidate.ID, Profile: profile}, nil
		}
		return target{}, fmt.Errorf("managed node %q not found", specs.TargetID)
	}

	if specs.Profile == "" {
		return target{}, errors.New("pool extra_specs must set profile, or target_id to pin a node")
	}

	var candidates []node
	for _, candidate := range nodes {
		if candidate.Status != "online" {
			continue
		}
		for _, profile := range candidate.CompatibleProfiles {
			if profile == specs.Profile {
				candidates = append(candidates, candidate)
				break
			}
		}
	}
	if len(candidates) == 0 {
		return target{}, fmt.Errorf("no online node with profile %q", specs.Profile)
	}

	load, err := targetLoad(api)
	if err != nil {
		return target{}, err
	}
	picked := leastLoaded(candidates, load, instanceName)
	if err := checkArch(osArch, picked.Arch); err != nil {
		return target{}, err
	}
	return target{Kind: kind, ID: picked.ID, Profile: specs.Profile}, nil
}

// targetLoad counts live deployments per node ID.
func targetLoad(api *api) (map[string]int, error) {
	items, err := api.deployments()
	if err != nil {
		// A listing failure must not block placement; assume no load.
		return map[string]int{}, nil
	}
	load := map[string]int{}
	for _, item := range items {
		if item.State != "removed" {
			load[item.TargetID]++
		}
	}
	return load, nil
}

// leastLoaded picks the candidate with the fewest live deployments. Ties are
// broken by the instance name, so concurrent pools spread over equal nodes.
func leastLoaded(candidates []node, load map[string]int, instanceName string) node {
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })

	minLoad := -1
	for _, candidate := range candidates {
		if count := load[candidate.ID]; minLoad < 0 || count < minLoad {
			minLoad = count
		}
	}

	var tied []node
	for _, candidate := range candidates {
		if load[candidate.ID] == minLoad {
			tied = append(tied, candidate)
		}
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(instanceName))
	return tied[int(h.Sum32()%uint32(len(tied)))]
}

func getInstance(api *api, providerID string) error {
	dep, err := api.deployment(providerID)
	if errors.Is(err, errNotFound) {
		return emit(providerInstance{ProviderID: providerID, Status: "stopped"})
	}
	if err != nil {
		return fmt.Errorf("reading deployment %s: %w", providerID, err)
	}
	return emit(providerInstance{ProviderID: dep.ID, Name: dep.RequestKey, Status: statusOf(dep)})
}

func listInstances(api *api, poolID string) error {
	items, err := api.deployments()
	if err != nil {
		return fmt.Errorf("listing deployments: %w", err)
	}
	instances := make([]providerInstance, 0, len(items))
	for _, item := range items {
		if item.State == "removed" || !belongsToPool(item, poolID) {
			continue
		}
		instances = append(instances, providerInstance{
			ProviderID: item.ID,
			Name:       item.RequestKey,
			Status:     statusOf(item),
		})
	}
	return json.NewEncoder(os.Stdout).Encode(instances)
}

func deleteInstance(api *api, providerID string) error {
	dep, err := api.deployment(providerID)
	if errors.Is(err, errNotFound) {
		dep, err = api.deploymentByRequestKey(providerID)
	}
	if errors.Is(err, errNotFound) {
		return notFoundError{msg: fmt.Sprintf("deployment %q not found", providerID)}
	}
	if err != nil {
		return fmt.Errorf("reading deployment %q: %w", providerID, err)
	}
	if dep.State == "removed" {
		return nil
	}
	if err := api.remove(dep.ID); err != nil {
		return fmt.Errorf("removing deployment %s: %w", dep.ID, err)
	}
	return api.waitRemoved(dep.ID, removeTimeoutSeconds*time.Second)
}

func removeAllInstances(api *api, poolID string) error {
	items, err := api.deployments()
	if err != nil {
		return fmt.Errorf("listing deployments: %w", err)
	}
	for _, item := range items {
		if item.State == "removed" || !belongsToPool(item, poolID) {
			continue
		}
		if err := api.remove(item.ID); err != nil {
			return fmt.Errorf("removing deployment %s: %w", item.ID, err)
		}
	}
	return nil
}

func startInstance(api *api, providerID string) error {
	if err := api.start(providerID); err != nil && !errors.Is(err, errNotFound) {
		return fmt.Errorf("starting deployment %s: %w", providerID, err)
	}
	return nil
}

func stopInstance(api *api, providerID string) error {
	if err := api.stop(providerID); err != nil && !errors.Is(err, errNotFound) {
		return fmt.Errorf("stopping deployment %s: %w", providerID, err)
	}
	return nil
}

// statusOf maps a V-Scheduler deployment state to a GARM provider status.
// GARM only accepts running, stopped, error and unknown from a provider.
func statusOf(dep deployment) string {
	switch dep.State {
	case "pending", "running":
		return "running"
	case "stopped", "stopping", "deleting", "removed":
		return "stopped"
	default:
		return "error"
	}
}

func belongsToPool(dep deployment, poolID string) bool {
	if poolID == "" {
		return true
	}
	value, _ := dep.Config["pool_id"].(string)
	return value == poolID
}
