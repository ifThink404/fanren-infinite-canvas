package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tigerowo/infinite-canvas/model"
)

func TestNormalizeFanrenImageJobBody(t *testing.T) {
	body, err := normalizeFanrenImageJobBody([]byte(`{"model":"gpt-image-2","stream":true,"response_format":"b64_json","n":2}`), "application/json", "gpt-image-2", "a quiet mountain")
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"gpt-image-2","n":2}`, string(body))
}

func TestNormalizeFanrenImageJobBodyRejectsOversizedBatch(t *testing.T) {
	_, err := normalizeFanrenImageJobBody([]byte(`{"model":"gpt-image-2","n":5}`), "application/json", "gpt-image-2", "mountain")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "最多支持")
}

func TestParseFanrenImageJobResponse(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantID     string
		wantStatus string
		wantAssets int
	}{
		{name: "queued numeric id", body: `{"job":{"id":42,"status":"queued"}}`, wantID: "42", wantStatus: "queued"},
		{name: "running", body: `{"job":{"id":"job_1","status":"in_progress","progress":37}}`, wantID: "job_1", wantStatus: "processing"},
		{name: "success", body: `{"job":{"id":"job_1","status":"succeeded","assets":[{"proxy_url":"/p/img/1"}]}}`, wantID: "job_1", wantStatus: "succeeded", wantAssets: 1},
		{name: "failure", body: `{"job":{"id":"job_1","status":"failed","error_message":"upstream rejected"}}`, wantID: "job_1", wantStatus: "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseFanrenImageJobResponse([]byte(tt.body))
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, result.ID)
			assert.Equal(t, tt.wantStatus, result.Status)
			assert.Len(t, result.Assets, tt.wantAssets)
		})
	}
}

func TestCallFanrenImageJobUsesOpenAIPathAndAuthorization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/images/jobs", r.URL.Path)
		assert.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		payload, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, `{"model":"gpt-image-2"}`, string(payload))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"job":{"id":"job_1","status":"queued"}}`))
	}))
	defer server.Close()

	payload, status, err := callFanrenImageJob(model.ModelChannel{BaseURL: server.URL, APIKey: "sk-test", Timeout: 5}, http.MethodPost, "/images/jobs", []byte(`{"model":"gpt-image-2"}`))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.True(t, strings.Contains(string(payload), `"job"`))
}

func TestAbsoluteFanrenImageAssetURL(t *testing.T) {
	assert.Equal(t, "https://fanrenapi.com/p/img/1", absoluteFanrenImageAssetURL("https://fanrenapi.com/v1", "/p/img/1"))
	assert.Equal(t, "https://cdn.example.com/image.png", absoluteFanrenImageAssetURL("https://fanrenapi.com", "https://cdn.example.com/image.png"))
}
