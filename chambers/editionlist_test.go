package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEditionListNormalize(t *testing.T) {
	tests := []struct {
		name string
		fn   func(string) string
		in   string
		want string
	}{
		{"sort defaults to LLM first", normalizeEditionListSort, "", "llm"},
		{"unknown sort", normalizeEditionListSort, "c.cites; DROP", "llm"},
		{"sort by year", normalizeEditionListSort, "year", "year"},
		{"show defaults to all", normalizeEditionListShow, "", ""},
		{"unknown show", normalizeEditionListShow, "x", ""},
		{"show neither", normalizeEditionListShow, "neither", "neither"},
		{"jurisdiction defaults to both", normalizeEditionListJurisdiction, "", ""},
		{"unknown jurisdiction", normalizeEditionListJurisdiction, "both", ""},
		{"jurisdiction uk", normalizeEditionListJurisdiction, "uk", "uk"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.fn(tt.in))
		})
	}
}

func TestEditionListItem(t *testing.T) {
	tests := []struct {
		name        string
		item        EditionListItem
		hasPipeline bool
		hasLLM      bool
		complete    bool
		linked      string
		llmLinked   string
	}{
		{"neither", EditionListItem{Pages: 60}, false, false, false, "—", "—"},
		{"pipeline only", EditionListItem{Pages: 100, Cites: 200, Linked: 50}, true, false, false, "25%", "—"},
		{"read in part", EditionListItem{Pages: 100, Cites: 200, Linked: 50, LLMPages: 30, LLMCites: 10, LLMLinked: 9}, true, true, false, "25%", "90%"},
		{"read in full", EditionListItem{Pages: 100, LLMPages: 100, LLMCites: 4, LLMLinked: 4}, false, true, true, "—", "100%"},
		{"read, no citations found", EditionListItem{Pages: 10, LLMPages: 10}, false, true, true, "—", "—"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.hasPipeline, tt.item.HasPipeline())
			require.Equal(t, tt.hasLLM, tt.item.HasLLM())
			require.Equal(t, tt.complete, tt.item.LLMComplete())
			require.Equal(t, tt.linked, tt.item.LinkedPct())
			require.Equal(t, tt.llmLinked, tt.item.LLMLinkedPct())
		})
	}
}
