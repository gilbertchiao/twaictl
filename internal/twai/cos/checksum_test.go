package cos

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsPlainMD5ETag(t *testing.T) {
	assert.True(t, IsPlainMD5ETag(`"5d41402abc4b2a76b9719d911017c592"`))
	assert.True(t, IsPlainMD5ETag(`5d41402abc4b2a76b9719d911017c592`))
	assert.False(t, IsPlainMD5ETag(`"5d41402abc4b2a76b9719d911017c592-3"`), "multipart ETag")
	assert.False(t, IsPlainMD5ETag(``))
	assert.False(t, IsPlainMD5ETag(`"not-hex"`))
}

func TestFileMD5(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o644))
	sum, err := FileMD5(path)
	require.NoError(t, err)
	assert.Equal(t, "5d41402abc4b2a76b9719d911017c592", sum)

	_, err = FileMD5(filepath.Join(t.TempDir(), "missing"))
	assert.Error(t, err)
}
