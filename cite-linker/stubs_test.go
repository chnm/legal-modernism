package main

import (
	"testing"

	"github.com/lmullen/legal-modernism/go/citations"
	"github.com/stretchr/testify/assert"
)

func TestStubIndexLookup(t *testing.T) {
	ix := stubIndex{"1 Toth 123": {}, "34 L.T. 100": {}}

	// The first matching form wins, in the order given.
	cite, ok := ix.lookup([]string{"Toth 123", "1 Toth 123"})
	assert.True(t, ok)
	assert.Equal(t, "1 Toth 123", cite)

	_, ok = ix.lookup([]string{"34 L.T. 101"})
	assert.False(t, ok)

	_, ok = ix.lookup(nil)
	assert.False(t, ok)

	// A nil index is the state before the registry exists; it must be safe to
	// probe and must never match.
	var none stubIndex
	_, ok = none.lookup([]string{"34 L.T. 100"})
	assert.False(t, ok)
}

func TestLinkStub(t *testing.T) {
	r := &citations.LinkResult{}
	got := linkStub(r, "34 L.T. 100")
	assert.Same(t, r, got)
	assert.Equal(t, citations.StatusLinkedStub, got.Status)
	assert.Equal(t, citations.TierStubDirect, got.MatchTier)
	if assert.NotNil(t, got.StubCite) {
		assert.Equal(t, "34 L.T. 100", *got.StubCite)
	}
	if assert.NotNil(t, got.CiteLinked) {
		assert.Equal(t, "34 L.T. 100", *got.CiteLinked)
	}
}
