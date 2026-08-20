package ts11client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRegistry starts an httptest server serving a discovery index
// with the given credentials, plus each credential's format documents at
// the URLs the index itself declares. Mirrors the real shape confirmed
// against registry.siros.org's own .well-known/vctm-registry.json.
func newTestRegistry(t *testing.T, credentials []testCredential) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	index := map[string]any{"version": "2.0", "credentials": []map[string]any{}}
	creds := index["credentials"].([]map[string]any)
	for _, c := range credentials {
		formats := map[string]any{}
		if c.vctmBody != "" {
			path := "/" + c.org + "/" + c.slug + ".vctm.json"
			formats["vctm"] = map[string]string{"url": srv.URL + path, "type": "application/json"}
			body := c.vctmBody
			mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(body))
			})
		}
		if c.mdocBody != "" {
			path := "/" + c.org + "/" + c.slug + ".mdoc.json"
			formats["mdoc"] = map[string]string{"url": srv.URL + path, "type": "application/json"}
			body := c.mdocBody
			mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(body))
			})
		}
		creds = append(creds, map[string]any{
			"vct":          c.vct,
			"name":         c.slug,
			"organization": c.org,
			"formats":      formats,
		})
	}
	index["credentials"] = creds

	mux.HandleFunc("/.well-known/vctm-registry.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(index)
	})
	return srv
}

type testCredential struct {
	org, slug, vct string
	vctmBody       string
	mdocBody       string
}

func TestMultiRegistryClient_ResolveVCT_SingleRegistry(t *testing.T) {
	srv := newTestRegistry(t, []testCredential{
		{org: "sunet", slug: "ehic", vct: "https://example.com/ehic", vctmBody: `{"vct":"https://example.com/ehic","name":"EHIC"}`},
	})

	c, err := New(Config{Registries: []RegistryConfig{{BaseURL: srv.URL}}})
	require.NoError(t, err)

	res, err := c.ResolveVCT(context.Background(), "https://example.com/ehic")
	require.NoError(t, err)
	assert.Equal(t, srv.URL, res.Source)
	assert.Contains(t, string(res.Data), "EHIC")
}

func TestMultiRegistryClient_ResolveVCT_NotFound(t *testing.T) {
	srv := newTestRegistry(t, nil)
	c, err := New(Config{Registries: []RegistryConfig{{BaseURL: srv.URL}}})
	require.NoError(t, err)

	_, err = c.ResolveVCT(context.Background(), "urn:unknown")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMultiRegistryClient_ResolveDoctype(t *testing.T) {
	srv := newTestRegistry(t, []testCredential{
		{org: "sunet", slug: "mdl", vct: "urn:demo:mdl", mdocBody: `{"format":"mso_mdoc","doctype":"org.iso.18013.5.1.mDL"}`},
	})

	c, err := New(Config{Registries: []RegistryConfig{{BaseURL: srv.URL}}})
	require.NoError(t, err)

	res, err := c.ResolveDoctype(context.Background(), "org.iso.18013.5.1.mDL")
	require.NoError(t, err)
	assert.Equal(t, srv.URL, res.Source)
	assert.Contains(t, string(res.Data), "mDL")
}

func TestMultiRegistryClient_HappyEyeballs_FastestWins(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(slow.Close)

	fast := newTestRegistry(t, []testCredential{
		{org: "fast", slug: "cred", vct: "urn:race:cred", vctmBody: `{"vct":"urn:race:cred","name":"Fast"}`},
	})

	c, err := New(Config{Registries: []RegistryConfig{
		{BaseURL: slow.URL, Timeout: time.Second},
		{BaseURL: fast.URL, Timeout: time.Second},
	}})
	require.NoError(t, err)

	start := time.Now()
	res, err := c.ResolveVCT(context.Background(), "urn:race:cred")
	elapsed := time.Since(start)
	require.NoError(t, err)
	assert.Equal(t, fast.URL, res.Source)
	assert.Less(t, elapsed, 150*time.Millisecond, "should return as soon as the fast registry answers, not wait for the slow one")
}

func TestMultiRegistryClient_HappyEyeballs_SlowRegistryHasIt(t *testing.T) {
	// The fast registry doesn't have this vct; the slow one does. The
	// race must not give up just because one registry answered quickly
	// with "not found" - it should still wait for and use the slower hit.
	fastMiss := newTestRegistry(t, nil)
	slowHit := newTestRegistry(t, []testCredential{
		{org: "slow", slug: "cred", vct: "urn:race:only-slow-has-it", vctmBody: `{"vct":"urn:race:only-slow-has-it","name":"Slow"}`},
	})

	c, err := New(Config{Registries: []RegistryConfig{
		{BaseURL: fastMiss.URL},
		{BaseURL: slowHit.URL},
	}})
	require.NoError(t, err)

	res, err := c.ResolveVCT(context.Background(), "urn:race:only-slow-has-it")
	require.NoError(t, err)
	assert.Equal(t, slowHit.URL, res.Source)
}

func TestMultiRegistryClient_AllRegistriesMiss(t *testing.T) {
	a := newTestRegistry(t, nil)
	b := newTestRegistry(t, nil)
	c, err := New(Config{Registries: []RegistryConfig{{BaseURL: a.URL}, {BaseURL: b.URL}}})
	require.NoError(t, err)

	_, err = c.ResolveVCT(context.Background(), "urn:nowhere")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMultiRegistryClient_LocalAlwaysWinsOverRegistry(t *testing.T) {
	srv := newTestRegistry(t, []testCredential{
		{org: "remote", slug: "cred", vct: "urn:shared", vctmBody: `{"vct":"urn:shared","name":"FromRegistry"}`},
	})
	local, err := NewLocalSource(LocalEntry{VCT: "urn:shared", Data: []byte(`{"vct":"urn:shared","name":"FromLocal"}`)})
	require.NoError(t, err)

	c, err := New(Config{Local: local, Registries: []RegistryConfig{{BaseURL: srv.URL}}})
	require.NoError(t, err)

	res, err := c.ResolveVCT(context.Background(), "urn:shared")
	require.NoError(t, err)
	assert.Equal(t, "local", res.Source)
	assert.Contains(t, string(res.Data), "FromLocal")
}

func TestMultiRegistryClient_LocalMissFallsThroughToRegistry(t *testing.T) {
	srv := newTestRegistry(t, []testCredential{
		{org: "remote", slug: "cred", vct: "urn:only-remote", vctmBody: `{"vct":"urn:only-remote"}`},
	})
	local, err := NewLocalSource(LocalEntry{VCT: "urn:something-else", Data: []byte(`{}`)})
	require.NoError(t, err)

	c, err := New(Config{Local: local, Registries: []RegistryConfig{{BaseURL: srv.URL}}})
	require.NoError(t, err)

	res, err := c.ResolveVCT(context.Background(), "urn:only-remote")
	require.NoError(t, err)
	assert.Equal(t, srv.URL, res.Source)
}

func TestMultiRegistryClient_IndexCachedAcrossQueries(t *testing.T) {
	var fetchCount atomic.Int64
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/.well-known/vctm-registry.json", func(w http.ResponseWriter, r *http.Request) {
		fetchCount.Add(1)
		_, _ = w.Write([]byte(`{"version":"2.0","credentials":[{"vct":"urn:cached","formats":{"vctm":{"url":"` + srv.URL + `/cached.vctm.json","type":"application/json"}}}]}`))
	})
	mux.HandleFunc("/cached.vctm.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"vct":"urn:cached"}`))
	})

	c, err := New(Config{Registries: []RegistryConfig{{BaseURL: srv.URL}}})
	require.NoError(t, err)

	for range 5 {
		_, err := c.ResolveVCT(context.Background(), "urn:cached")
		require.NoError(t, err)
	}
	assert.Equal(t, int64(1), fetchCount.Load(), "the discovery index must be fetched once and cached, not once per query")
}

func TestMultiRegistryClient_IndexRefreshesAfterInterval(t *testing.T) {
	var fetchCount atomic.Int64
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/.well-known/vctm-registry.json", func(w http.ResponseWriter, r *http.Request) {
		fetchCount.Add(1)
		_, _ = w.Write([]byte(`{"version":"2.0","credentials":[]}`))
	})

	c, err := New(Config{
		Registries:      []RegistryConfig{{BaseURL: srv.URL}},
		RefreshInterval: 50 * time.Millisecond,
	})
	require.NoError(t, err)

	_, _ = c.ResolveVCT(context.Background(), "urn:whatever")
	assert.Equal(t, int64(1), fetchCount.Load())

	time.Sleep(100 * time.Millisecond)
	_, _ = c.ResolveVCT(context.Background(), "urn:whatever")
	assert.Equal(t, int64(2), fetchCount.Load(), "index should be re-fetched once RefreshInterval has elapsed")
}

func TestNew_RequiresAtLeastOneSource(t *testing.T) {
	_, err := New(Config{})
	assert.Error(t, err)
}

func TestNew_RejectsEmptyRegistryBaseURL(t *testing.T) {
	_, err := New(Config{Registries: []RegistryConfig{{BaseURL: ""}}})
	assert.Error(t, err)
}
