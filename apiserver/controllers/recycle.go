package controllers

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"

	gErrors "github.com/cloudbase/garm-provider-common/errors"

	"github.com/cloudbase/garm/params"
)

// RecyclePoolInstancesHandler replaces the selected instances of a pool.
func (a *APIController) RecyclePoolInstancesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	vars := mux.Vars(r)
	poolID, ok := vars["poolID"]
	if !ok {
		handleError(ctx, w, gErrors.NewBadRequestError("no pool ID specified"))
		return
	}

	param := params.RecyclePoolInstancesParams{}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&param); err != nil && !errors.Is(err, io.EOF) {
			handleError(ctx, w, gErrors.NewBadRequestError("invalid post body: %s", err))
			return
		}
	}

	task, err := a.r.RecyclePoolInstances(ctx, poolID, param)
	if err != nil {
		handleError(ctx, w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(task); err != nil {
		slog.With(slog.Any("error", err)).ErrorContext(ctx, "failed to encode response")
	}
}

// GetRecycleTaskHandler returns the progress of a pool instance replacement.
func (a *APIController) GetRecycleTaskHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	vars := mux.Vars(r)
	taskID, ok := vars["taskID"]
	if !ok {
		handleError(ctx, w, gErrors.NewBadRequestError("no task ID specified"))
		return
	}

	task, err := a.r.GetRecycleTask(ctx, taskID)
	if err != nil {
		handleError(ctx, w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(task); err != nil {
		slog.With(slog.Any("error", err)).ErrorContext(ctx, "failed to encode response")
	}
}
