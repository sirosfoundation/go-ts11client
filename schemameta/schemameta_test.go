package schemameta

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractDoctype(t *testing.T) {
	doctype, err := ExtractDoctype([]byte(`{"format":"mso_mdoc","doctype":"org.iso.18013.5.1.mDL"}`))
	require.NoError(t, err)
	assert.Equal(t, "org.iso.18013.5.1.mDL", doctype)
}

func TestExtractDoctype_MissingField(t *testing.T) {
	doctype, err := ExtractDoctype([]byte(`{"format":"mso_mdoc"}`))
	require.NoError(t, err)
	assert.Empty(t, doctype)
}

func TestExtractDoctype_InvalidJSON(t *testing.T) {
	_, err := ExtractDoctype([]byte(`not json`))
	assert.Error(t, err)
}

// TestIndex_UnmarshalsRealShape confirms Index/Entry decode a document
// matching the real shape served at
// https://registry.siros.org/.well-known/vctm-registry.json.
func TestIndex_UnmarshalsRealShape(t *testing.T) {
	const doc = `{
		"$schema": "https://registry.siros.org/schemas/vctm-registry.json",
		"name": "SIROS Credential Registry",
		"url": "https://registry.siros.org",
		"version": "2.0",
		"credentials": [
			{
				"vct": "https://skatteverket.se/credentials/skv-address-credential",
				"name": "SkvAddressCredential",
				"organization": "skv-wallet-poc",
				"formats": {
					"mdoc": {"url": "https://registry.siros.org/skv-wallet-poc/skv-address-credential.mdoc.json", "type": "application/json"},
					"vctm": {"url": "https://registry.siros.org/skv-wallet-poc/skv-address-credential.vctm.json", "type": "application/json"}
				},
				"metadata": {
					"html": "https://registry.siros.org/skv-wallet-poc/skv-address-credential.html",
					"json": "https://registry.siros.org/skv-wallet-poc/skv-address-credential.vctm.json"
				},
				"source": {"repository": "https://github.com/skv-wallet-poc/skv-vctm-test.git", "branch": "main"}
			}
		]
	}`

	var idx Index
	require.NoError(t, json.Unmarshal([]byte(doc), &idx))
	require.Len(t, idx.Credentials, 1)

	e := idx.Credentials[0]
	assert.Equal(t, "https://skatteverket.se/credentials/skv-address-credential", e.VCT)
	assert.Equal(t, "skv-wallet-poc", e.Organization)
	assert.Equal(t, "https://registry.siros.org/skv-wallet-poc/skv-address-credential.mdoc.json", e.Formats[FormatMDOC].URL)
	assert.Equal(t, "https://registry.siros.org/skv-wallet-poc/skv-address-credential.vctm.json", e.Formats[FormatVCTM].URL)
	assert.Equal(t, "https://github.com/skv-wallet-poc/skv-vctm-test.git", e.Source.Repository)
}
