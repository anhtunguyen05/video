package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"video/services/api/internal/platform/ids"
	"video/services/api/internal/platform/observability"
)

func TestRequestIDPropagatesToContext(t *testing.T) {
	for _, incoming := range []string{"a1161490-9da4-4b0d-93cf-2a86907f3304", "invalid"} {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("X-Request-ID", incoming)
		recorder := httptest.NewRecorder()
		withRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := observability.RequestID(r.Context())
			if got != w.Header().Get("X-Request-ID") || !ids.IsUUID(got) {
				t.Errorf("context and response IDs differ or invalid: %q", got)
			}
			if ids.IsUUID(incoming) && got != incoming {
				t.Errorf("valid request ID not preserved")
			}
		})).ServeHTTP(recorder, req)
	}
}
