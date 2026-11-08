package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"video/services/api/internal/media/domain"
	platformauth "video/services/api/internal/platform/auth"
)

type fakeSubtitleLookup struct {
	projection subtitleProjection
	err        error
	calls      int
}

func (f *fakeSubtitleLookup) Get(_ context.Context, _, _ string, _ int) (subtitleProjection, error) {
	f.calls++
	return f.projection, f.err
}

func TestSubtitleStatusAndTrackAreOwnerScoped(t *testing.T) {
	const owner = "00000000-0000-4000-8000-000000000001"
	const videoID = "00000000-0000-4000-8000-000000000002"
	const key = "private/vi.vtt"
	lookup := &fakeSubtitleLookup{projection: subtitleProjection{Status: "READY", ObjectKey: sql.NullString{String: key, Valid: true}}}
	api := &Server{videos: &fakeVideoService{video: domain.Video{ID: videoID, OwnerID: owner, ProcessingVersion: 1}},
		currentUser: platformauth.StaticPrincipal{ID: owner}, subtitles: lookup,
		assetReader: fakeAssetSigner{objects: map[string][]byte{key: []byte("WEBVTT\n\n")}}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/videos/{videoId}/subtitles", api.getSubtitles)
	mux.HandleFunc("GET /api/v1/videos/{videoId}/subtitles/vi.vtt", api.getSubtitleTrack)
	for _, test := range []struct{ path, want, contentType string }{
		{"/api/v1/videos/" + videoID + "/subtitles", `"status":"READY"`, "application/json"},
		{"/api/v1/videos/" + videoID + "/subtitles/vi.vtt", "WEBVTT", "text/vtt"},
	} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
		if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), test.want) || !strings.Contains(recorder.Header().Get("Content-Type"), test.contentType) || recorder.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("%s: status=%d body=%s headers=%v", test.path, recorder.Code, recorder.Body, recorder.Header())
		}
	}
	api.currentUser = platformauth.StaticPrincipal{ID: "00000000-0000-4000-8000-000000000099"}
	before := lookup.calls
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/videos/"+videoID+"/subtitles/vi.vtt", nil))
	if recorder.Code != http.StatusNotFound || lookup.calls != before {
		t.Fatal("other owner could discover track")
	}
}

func TestSubtitleNotRequestedAndPendingHaveNoTrack(t *testing.T) {
	const owner = "00000000-0000-4000-8000-000000000001"
	const videoID = "00000000-0000-4000-8000-000000000002"
	lookup := &fakeSubtitleLookup{err: sql.ErrNoRows}
	api := &Server{videos: &fakeVideoService{video: domain.Video{ID: videoID, OwnerID: owner, ProcessingVersion: 1}}, currentUser: platformauth.StaticPrincipal{ID: owner}, subtitles: lookup}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/videos/{videoId}/subtitles", api.getSubtitles)
	mux.HandleFunc("GET /api/v1/videos/{videoId}/subtitles/vi.vtt", api.getSubtitleTrack)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/videos/"+videoID+"/subtitles", nil))
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), "NOT_REQUESTED") {
		t.Fatalf("status: %d %s", recorder.Code, recorder.Body)
	}
	lookup.err = nil
	lookup.projection.Status = "PENDING"
	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/videos/"+videoID+"/subtitles/vi.vtt", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("pending track status = %d", recorder.Code)
	}
}
