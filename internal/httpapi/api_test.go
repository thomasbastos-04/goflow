package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJSON(t *testing.T) {
	rec:=httptest.NewRecorder()
	JSON(rec,http.StatusCreated,map[string]string{"status":"ok"})
	require.Equal(t,http.StatusCreated,rec.Code)
	require.Contains(t,rec.Body.String(),`"status":"ok"`)
	require.Equal(t,"application/json",rec.Header().Get("Content-Type"))
}
