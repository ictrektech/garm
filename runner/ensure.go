// Copyright 2026 Cloudbase Solutions SRL
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	runnerErrors "github.com/cloudbase/garm-provider-common/errors"
	commonParams "github.com/cloudbase/garm-provider-common/params"

	"github.com/cloudbase/garm/auth"
	"github.com/cloudbase/garm/params"
)

// VOSGiteaEndpointName is the endpoint created for the Gitea running on the
// same VOS device.
const VOSGiteaEndpointName = "vos-gitea"

// EnsureGiteaEntity creates the forge entity, its pool and its webhook secret
// when they are missing. Repeated calls keep the existing objects.
func (r *Runner) EnsureGiteaEntity(ctx context.Context, param params.EnsureEntityParams) (params.EnsureEntityResult, error) {
	if param.Repo == "" {
		return r.EnsureOrganization(ctx, param)
	}
	return r.EnsureRepository(ctx, param)
}

// EnsureOrganization keeps the organization, its pool and its webhook secret.
func (r *Runner) EnsureOrganization(ctx context.Context, param params.EnsureEntityParams) (params.EnsureEntityResult, error) {
	if !auth.IsAdmin(ctx) {
		return params.EnsureEntityResult{}, runnerErrors.ErrUnauthorized
	}

	result := params.EnsureEntityResult{Created: []string{}, Existing: []string{}}

	creds, err := r.vosGiteaCredentials(ctx)
	if err != nil {
		return params.EnsureEntityResult{}, err
	}

	org, err := r.store.GetOrganization(ctx, param.Owner, creds.Endpoint.Name)
	switch {
	case errors.Is(err, runnerErrors.ErrNotFound):
		org, err = r.CreateOrganization(ctx, params.CreateOrgParams{
			Name:             param.Owner,
			CredentialsName:  creds.Name,
			WebhookSecret:    newWebhookSecret(),
			PoolBalancerType: params.PoolBalancerTypeRoundRobin,
			ForgeType:        params.GiteaEndpointType,
			AgentMode:        true,
		})
		if err != nil {
			return params.EnsureEntityResult{}, fmt.Errorf("error creating organization %s: %w", param.Owner, err)
		}
		result.Created = append(result.Created, "organization")
	case err != nil:
		return params.EnsureEntityResult{}, fmt.Errorf("error fetching organization %s: %w", param.Owner, err)
	default:
		result.Existing = append(result.Existing, "organization")
	}
	result.OrganizationID = org.ID

	entity, err := org.GetEntity()
	if err != nil {
		return params.EnsureEntityResult{}, fmt.Errorf("error reading organization entity: %w", err)
	}
	poolID, created, err := r.ensurePool(ctx, entity, param)
	if err != nil {
		return params.EnsureEntityResult{}, err
	}
	result.PoolID = poolID
	if created {
		result.Created = append(result.Created, "pool")
	} else {
		result.Existing = append(result.Existing, "pool")
	}

	result.WebhookSecret = org.WebhookSecret
	return result, nil
}

// EnsureRepository keeps the repository, its pool and its webhook secret.
func (r *Runner) EnsureRepository(ctx context.Context, param params.EnsureEntityParams) (params.EnsureEntityResult, error) {
	if !auth.IsAdmin(ctx) {
		return params.EnsureEntityResult{}, runnerErrors.ErrUnauthorized
	}

	result := params.EnsureEntityResult{Created: []string{}, Existing: []string{}}

	creds, err := r.vosGiteaCredentials(ctx)
	if err != nil {
		return params.EnsureEntityResult{}, err
	}

	repo, err := r.store.GetRepository(ctx, param.Owner, param.Repo, creds.Endpoint.Name)
	switch {
	case errors.Is(err, runnerErrors.ErrNotFound):
		repo, err = r.CreateRepository(ctx, params.CreateRepoParams{
			Owner:            param.Owner,
			Name:             param.Repo,
			CredentialsName:  creds.Name,
			WebhookSecret:    newWebhookSecret(),
			PoolBalancerType: params.PoolBalancerTypeRoundRobin,
			ForgeType:        params.GiteaEndpointType,
			AgentMode:        true,
		})
		if err != nil {
			return params.EnsureEntityResult{}, fmt.Errorf("error creating repository %s/%s: %w", param.Owner, param.Repo, err)
		}
		result.Created = append(result.Created, "repository")
	case err != nil:
		return params.EnsureEntityResult{}, fmt.Errorf("error fetching repository %s/%s: %w", param.Owner, param.Repo, err)
	default:
		result.Existing = append(result.Existing, "repository")
	}
	result.RepositoryID = repo.ID

	entity, err := repo.GetEntity()
	if err != nil {
		return params.EnsureEntityResult{}, fmt.Errorf("error reading repository entity: %w", err)
	}
	poolID, created, err := r.ensurePool(ctx, entity, param)
	if err != nil {
		return params.EnsureEntityResult{}, err
	}
	result.PoolID = poolID
	if created {
		result.Created = append(result.Created, "pool")
	} else {
		result.Existing = append(result.Existing, "pool")
	}

	result.WebhookSecret = repo.WebhookSecret
	return result, nil
}

// ensurePool returns the pool of the entity, creating it when the entity has
// none yet.
func (r *Runner) ensurePool(ctx context.Context, entity params.ForgeEntity, param params.EnsureEntityParams) (string, bool, error) {
	pools, err := r.store.ListEntityPools(ctx, entity)
	if err != nil {
		return "", false, fmt.Errorf("error listing pools: %w", err)
	}
	if len(pools) > 0 {
		return pools[0].ID, false, nil
	}

	poolParams := r.vosPoolParams(param)
	var pool params.Pool
	if entity.EntityType == params.ForgeEntityTypeOrganization {
		pool, err = r.CreateOrgPool(ctx, entity.ID, poolParams)
	} else {
		pool, err = r.CreateRepoPool(ctx, entity.ID, poolParams)
	}
	if err != nil {
		return "", false, fmt.Errorf("error creating pool: %w", err)
	}
	return pool.ID, true, nil
}

// vosGiteaCredentials returns the credentials registered for the local Gitea
// endpoint.
func (r *Runner) vosGiteaCredentials(ctx context.Context) (params.ForgeCredentials, error) {
	creds, err := r.store.ListGiteaCredentials(ctx)
	if err != nil {
		return params.ForgeCredentials{}, fmt.Errorf("error listing gitea credentials: %w", err)
	}
	for _, cred := range creds {
		if strings.EqualFold(cred.Endpoint.Name, VOSGiteaEndpointName) {
			return cred, nil
		}
	}
	return params.ForgeCredentials{}, runnerErrors.NewConflictError("no credentials for endpoint %s", VOSGiteaEndpointName)
}

// vosPoolParams returns the pool defaults of a VOS device. The provider comes
// from the configuration; the rest matches the image shipped with the device.
// shortcut: the image and flavor are fixed; edit the pool in GARM when the
// device ships another image.
func (r *Runner) vosPoolParams(param params.EnsureEntityParams) params.CreatePoolParams {
	providerName := "lxd"
	if len(r.config.Providers) > 0 && r.config.Providers[0].Name != "" {
		providerName = r.config.Providers[0].Name
	}
	maxRunners := param.MaxRunners
	if maxRunners == 0 {
		maxRunners = 2
	}
	return params.CreatePoolParams{
		ProviderName:           providerName,
		MaxRunners:             maxRunners,
		MinIdleRunners:         0,
		Image:                  "ubuntu:22.04",
		Flavor:                 "default",
		OSType:                 commonParams.Linux,
		OSArch:                 commonParams.Amd64,
		Tags:                   param.Labels,
		Enabled:                true,
		RunnerBootstrapTimeout: 20,
		Priority:               100,
	}
}

func newWebhookSecret() string {
	buf := make([]byte, 32)
	// crypto/rand does not fail on the platforms GARM runs on.
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
