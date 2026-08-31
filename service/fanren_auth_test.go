package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tigerowo/infinite-canvas/config"
	"github.com/tigerowo/infinite-canvas/model"
)

func TestCurrentAuthUserUsesFanrenSSOIdentityAndCache(t *testing.T) {
	previous := config.Cfg
	t.Cleanup(func() { config.Cfg = previous })

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "Bearer main-dashboard-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":178,"username":"dao-you","display_name":"道友","email":"dao@example.com","role":10,"status":1}}`))
	}))
	defer server.Close()

	config.Cfg.FanrenSSORequired = true
	config.Cfg.FanrenAuthBaseURL = server.URL

	user, ok := CurrentAuthUser("main-dashboard-token")
	require.True(t, ok)
	assert.Equal(t, "fanren:178", user.ID)
	assert.Equal(t, "dao-you", user.Username)
	assert.Equal(t, "道友", user.DisplayName)
	assert.Equal(t, model.UserRoleAdmin, user.Role)

	cached, ok := CurrentAuthUser("main-dashboard-token")
	require.True(t, ok)
	assert.Equal(t, user, cached)
	assert.Equal(t, 1, requestCount)
}
