package cos

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseURL(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		want   Location
		wantOK bool
	}{
		{"bucket only", "cos://bucket", Location{Bucket: "bucket"}, true},
		{"bucket with trailing slash", "cos://bucket/", Location{Bucket: "bucket"}, true},
		{"bucket and key", "cos://bucket/dir/file.txt", Location{Bucket: "bucket", Key: "dir/file.txt"}, true},
		{"prefix trailing slash preserved", "cos://bucket/prefix/", Location{Bucket: "bucket", Key: "prefix/"}, true},
		{"not cos scheme", "s3://bucket/key", Location{}, false},
		{"local path", "./file.txt", Location{}, false},
		{"empty", "", Location{}, false},
		{"scheme only", "cos://", Location{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ParseURL(c.input)
			assert.Equal(t, c.wantOK, ok)
			if c.wantOK {
				assert.Equal(t, c.want, got)
			}
		})
	}
}

func TestLocationString(t *testing.T) {
	assert.Equal(t, "cos://bucket", Location{Bucket: "bucket"}.String())
	assert.Equal(t, "cos://bucket/dir/file.txt", Location{Bucket: "bucket", Key: "dir/file.txt"}.String())
	assert.Equal(t, "cos://bucket/prefix/", Location{Bucket: "bucket", Key: "prefix/"}.String())
}

func TestIsRemote(t *testing.T) {
	assert.True(t, IsRemote("cos://bucket/key"))
	assert.True(t, IsRemote("cos://bucket"))
	assert.False(t, IsRemote("./local/path"))
	assert.False(t, IsRemote("s3://bucket/key"))
	assert.False(t, IsRemote(""))
}
