package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var errNotFound = errors.New("not found")

// httpError keeps the status code so callers can explain a rejection.
type httpError struct {
	method string
	path   string
	status int
	body   string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("%s %s returned %d: %s", e.method, e.path, e.status, e.body)
}

type api struct {
	base  string
	token string
	hc    *http.Client
}

func newAPI(cfg config) *api {
	return &api{
		base:  strings.TrimRight(cfg.BaseURL, "/"),
		token: cfg.Token,
		hc:    &http.Client{Timeout: cfg.timeout()},
	}
}

type node struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Status             string   `json:"status"`
	Profile            string   `json:"profile"`
	Arch               string   `json:"arch"`
	CompatibleProfiles []string `json:"compatible_profiles"`
}

type target struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Profile string `json:"profile"`
}

type deployment struct {
	ID         string         `json:"id"`
	RequestKey string         `json:"request_key"`
	TargetID   string         `json:"target_id"`
	TargetKind string         `json:"target_kind"`
	Profile    string         `json:"profile"`
	Host       string         `json:"host"`
	HostPort   int            `json:"host_port"`
	Image      string         `json:"image"`
	State      string         `json:"state"`
	LastError  *string        `json:"last_error"`
	Config     map[string]any `json:"config"`
}

type createRequest struct {
	RequestKey    string         `json:"request_key"`
	Target        target         `json:"target"`
	Image         string         `json:"image"`
	ContainerPort uint16         `json:"container_port"`
	CPUMillicores *uint32        `json:"cpu_millicores,omitempty"`
	MemoryMib     *uint32        `json:"memory_mib,omitempty"`
	Config        map[string]any `json:"config"`
}

func (a *api) do(method, path string, body, out any) error {
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding %s request: %w", path, err)
		}
		payload = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, a.base+path, payload)
	if err != nil {
		return fmt.Errorf("building %s %s: %w", method, path, err)
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := a.hc.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading %s %s response: %w", method, path, err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if resp.StatusCode >= 300 {
		return &httpError{method: method, path: path, status: resp.StatusCode, body: strings.TrimSpace(string(data))}
	}
	if out == nil {
		return nil
	}

	var envelope struct {
		Code string          `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("decoding %s %s response: %w", method, path, err)
	}
	if envelope.Code != "" && envelope.Code != "OK" {
		return fmt.Errorf("%s %s returned %s: %s", method, path, envelope.Code, envelope.Msg)
	}
	if len(envelope.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("decoding %s %s data: %w", method, path, err)
	}
	return nil
}

// garm reads the same answer to fill the pool form picker
// (runner/providertargets). Change both when the shape changes.
func (a *api) nodes() ([]node, error) {
	var data struct {
		Items []node `json:"items"`
	}
	if err := a.do(http.MethodGet, "/managed-nodes", nil, &data); err != nil {
		return nil, err
	}
	return data.Items, nil
}

func (a *api) deployments() ([]deployment, error) {
	var data struct {
		Items []deployment `json:"items"`
	}
	if err := a.do(http.MethodGet, "/backend-deployments", nil, &data); err != nil {
		return nil, err
	}
	return data.Items, nil
}

func (a *api) deployment(id string) (deployment, error) {
	var dep deployment
	err := a.do(http.MethodGet, "/backend-deployments/"+url.PathEscape(id), nil, &dep)
	return dep, err
}

// deploymentByRequestKey finds a deployment by the GARM instance name. GARM
// calls DeleteInstance with the instance name when CreateInstance failed before
// a provider ID was stored.
func (a *api) deploymentByRequestKey(name string) (deployment, error) {
	items, err := a.deployments()
	if err != nil {
		return deployment{}, err
	}
	for _, item := range items {
		if item.RequestKey == name && item.State != "removed" {
			return item, nil
		}
	}
	return deployment{}, errNotFound
}

func (a *api) create(req createRequest) (deployment, error) {
	var dep deployment
	err := a.do(http.MethodPost, "/backend-deployments", req, &dep)
	return dep, err
}

func (a *api) remove(id string) error {
	return a.do(http.MethodDelete, "/backend-deployments/"+url.PathEscape(id), nil, nil)
}

func (a *api) start(id string) error {
	return a.do(http.MethodPost, "/backend-deployments/"+url.PathEscape(id)+"/start", nil, nil)
}

func (a *api) stop(id string) error {
	return a.do(http.MethodPost, "/backend-deployments/"+url.PathEscape(id)+"/stop", nil, nil)
}

// waitRemoved polls until V-Scheduler confirms container removal. Removal is
// asynchronous: DELETE only marks the deployment and a background task runs it.
func (a *api) waitRemoved(id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		dep, err := a.deployment(id)
		switch {
		case errors.Is(err, errNotFound):
			return nil
		case err != nil:
			return err
		case dep.State == "removed" || dep.State == "stopped":
			return nil
		case dep.State == "cleanup_failed":
			return fmt.Errorf("deployment %s cleanup failed: %s", id, faultText(dep))
		case time.Now().After(deadline):
			return fmt.Errorf("deployment %s still %s after %s", id, dep.State, timeout)
		}
		time.Sleep(2 * time.Second)
	}
}

func faultText(dep deployment) string {
	if dep.LastError == nil {
		return "no detail reported"
	}
	return *dep.LastError
}
