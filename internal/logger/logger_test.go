package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	require.NotNil(t, New(false))
	require.NotNil(t, New(true))
}

func TestMethods_NoPanic(t *testing.T) {
	l := New(false)
	assert.NotPanics(t, func() {
		l.Debugf("debug %d", 1)
		l.Infof("info %s", "msg")
		l.Warnf("warn %v", true)
		l.Errorf("error %q", "oops")
	})
}

func TestWith(t *testing.T) {
	l := New(false)
	child := l.With("request_id", "abc123")
	require.NotNil(t, child)
	assert.NotPanics(t, func() { child.Infof("child works") })
}

func TestWith_Chained(t *testing.T) {
	l := New(true).With("svc", "game").With("room", "r1")
	require.NotNil(t, l)
	assert.NotPanics(t, func() { l.Infof("chained") })
}
