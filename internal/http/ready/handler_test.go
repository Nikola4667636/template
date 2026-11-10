package ready_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	api "github.com/Nikola4667636/template/internal/generated"
	"github.com/Nikola4667636/template/internal/http/ready"
)

type requestKey struct{}

func TestServeHTTP(t *testing.T) {
	tests := []struct {
		name       string
		pingErr    error
		noDB       bool
		cancelled  bool
		timeout    bool
		wantStatus int
		wantBody   api.HealthResponseStatus
	}{
		{name: "database ready", wantStatus: http.StatusOK, wantBody: api.Ok},
		{name: "database unavailable", pingErr: errors.New("database unavailable"), wantStatus: http.StatusServiceUnavailable, wantBody: api.Unavailable},
		{name: "database missing", noDB: true, wantStatus: http.StatusServiceUnavailable, wantBody: api.Unavailable},
		{name: "request cancelled", cancelled: true, wantStatus: http.StatusServiceUnavailable, wantBody: api.Unavailable},
		{name: "ping times out", timeout: true, wantStatus: http.StatusServiceUnavailable, wantBody: api.Unavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			db := NewMockpinger(ctrl)
			ctx, cancel := context.WithCancel(context.WithValue(t.Context(), requestKey{}, tt.name))
			defer cancel()
			if tt.cancelled {
				cancel()
			}
			timeout := time.Second
			if tt.timeout {
				timeout = 0
			}
			var pingCtx context.Context
			if !tt.noDB {
				db.EXPECT().Ping(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
					pingCtx = ctx
					if ctx.Value(requestKey{}) != tt.name {
						t.Error("request context value was not forwarded")
					}
					if _, ok := ctx.Deadline(); !ok {
						t.Error("ping context has no deadline")
					}
					if tt.timeout || tt.cancelled {
						<-ctx.Done()
						return ctx.Err()
					}
					return tt.pingErr
				})
			}
			h := ready.New(db, timeout)
			if tt.noDB {
				h = ready.New(nil, timeout)
			}
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/ready", nil).WithContext(ctx)

			h.ServeHTTP(recorder, req)

			if recorder.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
			var body api.HealthResponse
			if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Status != tt.wantBody {
				t.Errorf("body status = %q, want %q", body.Status, tt.wantBody)
			}
			if pingCtx != nil && pingCtx.Err() == nil {
				t.Error("ping context was not cancelled after the handler returned")
			}
		})
	}
}
