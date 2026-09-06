package vcs

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gilbertchiao/twaictl/internal/twai"
)

func TestGetImageDecodes(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/images/21/", 200, fixture(t, "image_detail"))
	s := newService(t, f)

	res, err := s.GetImage(context.Background(), 21)
	require.NoError(t, err)
	assert.Equal(t, "Ubuntu 22.04", res.Value.Name)
	assert.Equal(t, int64(3658462), res.Value.Server.Id)
	assert.JSONEq(t, string(fixture(t, "image_detail")), string(res.Raw))
}

func TestSaveImageOmitsEmptyFields(t *testing.T) {
	f := newFakeAPI(t)
	f.on("PUT", vcsBase+"/images/3658462/save/", 201, fixture(t, "image_saved"))
	s := newService(t, f)

	_, err := s.SaveImage(context.Background(), SaveImageInput{ServerID: 3658462, Name: "x"})
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(f.Requests()[0].Body, &body))
	assert.Equal(t, map[string]any{"name": "x"}, body)

	_, err = s.SaveImage(context.Background(), SaveImageInput{
		ServerID: 3658462, Name: "x", OS: "Linux", OSVersion: "22.04", Desc: "d",
	})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(f.Requests()[1].Body, &body))
	assert.Equal(t, map[string]any{"name": "x", "os": "Linux", "os_version": "22.04", "desc": "d"}, body)
}

func TestDeleteImageChecksStatus(t *testing.T) {
	f := newFakeAPI(t)
	f.on("DELETE", vcsBase+"/images/21/", 500, []byte(`{"message":"boom"}`))
	s := newService(t, f)

	err := s.DeleteImage(context.Background(), 21)
	require.Error(t, err)
	var apiErr *twai.APIError
	assert.ErrorAs(t, err, &apiErr)
}

func TestResolveImageIDByNameAndAmbiguity(t *testing.T) {
	f := newFakeAPI(t)
	f.on("GET", vcsBase+"/images/", 200, fixture(t, "images"))
	s := newService(t, f)

	id, err := s.ResolveImageID(context.Background(), 101, "Ubuntu 22.04")
	require.NoError(t, err)
	assert.Equal(t, int64(21), id)

	f2 := newFakeAPI(t)
	f2.on("GET", vcsBase+"/images/", 200, []byte(`[
		{"id":31,"name":"dup","platform":"p","is_public":false,"size":1,"status":"active","create_time":"2026-08-27T00:00:00Z","server":{"hostname":"a","id":1}},
		{"id":32,"name":"dup","platform":"p","is_public":false,"size":1,"status":"active","create_time":"2026-08-27T00:00:00Z","server":{"hostname":"b","id":2}}
	]`))
	s2 := newService(t, f2)
	_, err = s2.ResolveImageID(context.Background(), 101, "dup")
	assert.ErrorContains(t, err, "31")
	assert.ErrorContains(t, err, "32")
}
