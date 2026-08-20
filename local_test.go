package ts11client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalSource_ResolveVCT(t *testing.T) {
	l, err := NewLocalSource(LocalEntry{VCT: "urn:demo:1", Data: []byte(`{"vct":"urn:demo:1"}`)})
	require.NoError(t, err)

	res, err := l.ResolveVCT(context.Background(), "urn:demo:1")
	require.NoError(t, err)
	assert.Equal(t, "local", res.Source)
	assert.Equal(t, []byte(`{"vct":"urn:demo:1"}`), res.Data)

	_, err = l.ResolveVCT(context.Background(), "urn:demo:missing")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestLocalSource_ResolveDoctype(t *testing.T) {
	l, err := NewLocalSource(LocalEntry{Doctype: "org.iso.18013.5.1.mDL", Data: []byte(`{"doctype":"org.iso.18013.5.1.mDL"}`)})
	require.NoError(t, err)

	res, err := l.ResolveDoctype(context.Background(), "org.iso.18013.5.1.mDL")
	require.NoError(t, err)
	assert.Equal(t, "local", res.Source)

	_, err = l.ResolveDoctype(context.Background(), "org.iso.other")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestLocalSource_NilReceiverAlwaysMisses(t *testing.T) {
	var l *LocalSource
	_, err := l.ResolveVCT(context.Background(), "urn:anything")
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = l.ResolveDoctype(context.Background(), "anything")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestNewLocalSource_RejectsEntryWithBothKeys(t *testing.T) {
	_, err := NewLocalSource(LocalEntry{VCT: "urn:x", Doctype: "org.x", Data: []byte(`{}`)})
	assert.Error(t, err)
}

func TestNewLocalSource_RejectsEntryWithNeitherKey(t *testing.T) {
	_, err := NewLocalSource(LocalEntry{Data: []byte(`{}`)})
	assert.Error(t, err)
}
