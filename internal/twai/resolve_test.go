package twai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type named struct {
	ID   int64
	Name string
}

func TestIsNumericRef(t *testing.T) {
	assert.True(t, IsNumericRef("123"))
	assert.False(t, IsNumericRef(""))
	assert.False(t, IsNumericRef("12a"))
	assert.False(t, IsNumericRef("-1"))
}

func TestResolveRef(t *testing.T) {
	items := []named{{1, "alpha"}, {2, "beta"}, {3, "beta"}}
	name := func(n named) string { return n.Name }
	id := func(n named) int64 { return n.ID }

	got, err := ResolveRef("site", "42", items, name, id)
	require.NoError(t, err)
	assert.Equal(t, int64(42), got, "純數字直接視為 id，不查清單")

	got, err = ResolveRef("site", "alpha", items, name, id)
	require.NoError(t, err)
	assert.Equal(t, int64(1), got)

	_, err = ResolveRef("site", "gamma", items, name, id)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "找不到 site")
	assert.Contains(t, err.Error(), "gamma")

	_, err = ResolveRef("site", "beta", items, name, id)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2, 3")
	assert.Contains(t, err.Error(), "改用 id")
}
