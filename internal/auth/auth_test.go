package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPasswordAndJWT(t *testing.T) {
	hash,err:=Hash("strong-password")
	require.NoError(t,err)
	require.NoError(t,Compare(hash,"strong-password"))
	require.Error(t,Compare(hash,"wrong"))

	m:=New("secret",time.Hour)
	token,err:=m.Generate("user-123")
	require.NoError(t,err)
	id,err:=m.Parse(token)
	require.NoError(t,err)
	require.Equal(t,"user-123",id)
}
