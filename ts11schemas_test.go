package ts11client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchTS11Schemas_SinglePageMultiFormat(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/api/v1/schemas.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"data": [{
				"id": "abc-123",
				"version": "1.0",
				"attestationLoS": "iso_18045_high",
				"supportedFormats": ["dc+sd-jwt", "mso_mdoc"],
				"schemaURIs": [
					{"formatIdentifier": "dc+sd-jwt", "uri": "` + srv.URL + `/cred.vctm.json"},
					{"formatIdentifier": "mso_mdoc", "uri": "` + srv.URL + `/cred.mdoc.json"}
				],
				"rulebookURI": "` + srv.URL + `/cred/rulebook.html"
			}],
			"total": 1, "limit": 100, "offset": 0
		}`))
	})
	mux.HandleFunc("/cred.vctm.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"vct":"urn:demo:cred"}`))
	})
	mux.HandleFunc("/cred.mdoc.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"doctype":"org.demo.cred"}`))
	})

	docs, err := FetchTS11Schemas(context.Background(), srv.Client(), srv.URL+"/api/v1/schemas.json")
	require.NoError(t, err)
	require.Len(t, docs, 2)

	byFormat := map[string]TS11Document{}
	for _, d := range docs {
		byFormat[d.FormatIdentifier] = d
	}
	assert.Contains(t, string(byFormat["dc+sd-jwt"].Data), "urn:demo:cred")
	assert.Contains(t, string(byFormat["mso_mdoc"].Data), "org.demo.cred")
	assert.Equal(t, "abc-123", byFormat["dc+sd-jwt"].Schema.ID)
	assert.Equal(t, "iso_18045_high", byFormat["dc+sd-jwt"].Schema.AttestationLoS)
}

func TestFetchTS11Schemas_FollowsOffsetPagination(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/doc1.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"vct":"urn:demo:1"}`))
	})
	mux.HandleFunc("/doc2.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"vct":"urn:demo:2"}`))
	})
	mux.HandleFunc("/api/v1/schemas.json", func(w http.ResponseWriter, r *http.Request) {
		offset := r.URL.Query().Get("offset")
		if offset == "" {
			_, _ = w.Write([]byte(`{
				"data": [{"id":"s1","schemaURIs":[{"formatIdentifier":"dc+sd-jwt","uri":"` + srv.URL + `/doc1.json"}]}],
				"total": 2, "limit": 1, "offset": 0
			}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"data": [{"id":"s2","schemaURIs":[{"formatIdentifier":"dc+sd-jwt","uri":"` + srv.URL + `/doc2.json"}]}],
			"total": 2, "limit": 1, "offset": 1
		}`))
	})

	docs, err := FetchTS11Schemas(context.Background(), srv.Client(), srv.URL+"/api/v1/schemas.json")
	require.NoError(t, err)
	require.Len(t, docs, 2)
	assert.Contains(t, string(docs[0].Data), "urn:demo:1")
	assert.Contains(t, string(docs[1].Data), "urn:demo:2")
}

func TestFetchTS11Schemas_FollowsLegacyNextPagination(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/doc1.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"vct":"urn:legacy:1"}`))
	})
	mux.HandleFunc("/doc2.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"vct":"urn:legacy:2"}`))
	})
	mux.HandleFunc("/page1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"schemas": [{"id":"s1","schemaURIs":[{"formatIdentifier":"dc+sd-jwt","uri":"` + srv.URL + `/doc1.json"}]}],
			"next": "` + srv.URL + `/page2"
		}`))
	})
	mux.HandleFunc("/page2", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"schemas": [{"id":"s2","schemaURIs":[{"formatIdentifier":"dc+sd-jwt","uri":"` + srv.URL + `/doc2.json"}]}]
		}`))
	})

	docs, err := FetchTS11Schemas(context.Background(), srv.Client(), srv.URL+"/page1")
	require.NoError(t, err)
	require.Len(t, docs, 2)
	assert.Contains(t, string(docs[0].Data), "urn:legacy:1")
	assert.Contains(t, string(docs[1].Data), "urn:legacy:2")
}

func TestFetchTS11Schemas_SkipsFailedDocumentButKeepsOthers(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/ok.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"vct":"urn:ok"}`))
	})
	mux.HandleFunc("/missing.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/api/v1/schemas.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"data": [
				{"id":"broken","schemaURIs":[{"formatIdentifier":"dc+sd-jwt","uri":"` + srv.URL + `/missing.json"}]},
				{"id":"good","schemaURIs":[{"formatIdentifier":"dc+sd-jwt","uri":"` + srv.URL + `/ok.json"}]}
			],
			"total": 2, "limit": 100, "offset": 0
		}`))
	})

	docs, err := FetchTS11Schemas(context.Background(), srv.Client(), srv.URL+"/api/v1/schemas.json")
	require.NoError(t, err)
	require.Len(t, docs, 1, "the failed document must be skipped, not fail the whole fetch")
	assert.Contains(t, string(docs[0].Data), "urn:ok")
}

func TestFetchTS11Schemas_FirstPageFetchFailureIsFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	_, err := FetchTS11Schemas(context.Background(), srv.Client(), srv.URL+"/api/v1/schemas.json")
	assert.Error(t, err)
}
