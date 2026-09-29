package access

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"infra.local/platform/internal/usage"
)

func TestUsageAPIIsTenantScopedAndProtectsLimitChanges(t *testing.T) {
	s := testStore(t)
	owner, session := principal(t, s, "owner")
	_, other := principal(t, s, "other")
	project, err := s.CreateProject(context.Background(), owner.OrganizationID, "Usage API")
	if err != nil {
		t.Fatal(err)
	}
	app := &App{store: s, config: Config{Origin: "http://localhost:5173"}}
	h := app.Routes()
	request := func(method, path, body, token, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/v1/projects/"+project.ID+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		if token != "" {
			r.AddCookie(&http.Cookie{Name: "infra_session", Value: token})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, tc := range []struct {
		method, path, body, token, origin string
		status                            int
	}{
		{"GET", "/usage", "", "", "", 401},
		{"GET", "/usage", "", other, "", 404},
		{"PUT", "/limits", `{"dailyUnits":1,"minuteRequests":1}`, other, app.config.Origin, 404},
		{"PUT", "/limits", `{"dailyUnits":1,"minuteRequests":1}`, session, "https://attacker.test", 403},
		{"PUT", "/limits", `{"dailyUnits":1}`, session, app.config.Origin, 400},
		{"PUT", "/limits", `{"dailyUnits":1,"minuteRequests":null}`, session, app.config.Origin, 400},
		{"PUT", "/limits", `{"dailyUnits":100001,"minuteRequests":1}`, session, app.config.Origin, 400},
		{"PUT", "/limits", `{"dailyUnits":1,"minuteRequests":1,"usedUnits":0}`, session, app.config.Origin, 400},
		{"PUT", "/limits", `{"dailyUnits":20,"minuteRequests":5}`, session, app.config.Origin, 200},
	} {
		w := request(tc.method, tc.path, tc.body, tc.token, tc.origin)
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.body, w.Code, w.Body.String())
		}
	}
	w := request("GET", "/usage", "", session, "")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var summary usage.Summary
	if err := json.Unmarshal(w.Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Limits.DailyUnits != 20 || summary.RemainingUnits != 20 || len(summary.Days) != 0 {
		t.Fatal(summary)
	}
}
