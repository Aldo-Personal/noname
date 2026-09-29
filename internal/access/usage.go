package access

import (
	"errors"
	"net/http"

	"infra.local/platform/internal/usage"
)

func usageFailure(w http.ResponseWriter, err error) {
	if errors.Is(err, usage.ErrNotFound) {
		failure(w, ErrNotFound)
	} else if errors.Is(err, usage.ErrInvalid) {
		failure(w, ErrInvalid)
	} else {
		failure(w, err)
	}
}
func (a *App) usageRoutes(mux *http.ServeMux) {
	store := &usage.Store{Pool: a.store.Pool}
	mux.HandleFunc("GET /v1/projects/{project}/usage", a.authorized(func(w http.ResponseWriter, r *http.Request, p Principal) {
		summary, err := store.Summary(r.Context(), p.OrganizationID, r.PathValue("project"))
		if err != nil {
			usageFailure(w, err)
			return
		}
		write(w, 200, summary)
	}))
	mux.HandleFunc("PUT /v1/projects/{project}/limits", a.authorized(func(w http.ResponseWriter, r *http.Request, p Principal) {
		var body struct {
			DailyUnits     *int64 `json:"dailyUnits"`
			MinuteRequests *int64 `json:"minuteRequests"`
		}
		if err := decode(w, r, &body); err != nil || body.DailyUnits == nil || body.MinuteRequests == nil {
			failure(w, ErrInvalid)
			return
		}
		limits := usage.Limits{DailyUnits: *body.DailyUnits, MinuteRequests: *body.MinuteRequests}
		if err := store.SetLimits(r.Context(), p.OrganizationID, r.PathValue("project"), limits); err != nil {
			usageFailure(w, err)
			return
		}
		write(w, 200, limits)
	}))
}
