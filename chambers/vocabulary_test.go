package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/lmullen/legal-modernism/go/citations"
	"github.com/stretchr/testify/require"
)

// TestVocabularyCoversLinker reads every Tier* and Status* constant out of
// go/citations (the same way linker_tier_constraint_test.go does) and requires
// the vocabulary to know each one, so a tier the linker starts writing reaches
// the pages with a colour and a gloss rather than unlabelled.
func TestVocabularyCoversLinker(t *testing.T) {
	tiers := linkerConstants(t, "Tier")
	require.NotEmpty(t, tiers, "found no Tier* constants; has the naming changed?")
	for name, value := range tiers {
		info, ok := tierByKey[value]
		require.True(t, ok, "%s = %q has no vocabulary entry", name, value)
		require.NotEmpty(t, info.Gloss, "%s has no gloss", value)
		require.Regexp(t, `^#[0-9a-f]{6}$`, info.Color, "%s has no colour", value)
		require.Contains(t, statusLabels, info.Status, "%s belongs to an unlabelled status", value)
	}

	statuses := linkerConstants(t, "Status")
	require.NotEmpty(t, statuses)
	for name, value := range statuses {
		require.Contains(t, statusLabels, value, "%s = %q has no label", name, value)
		require.Contains(t, statusOrder, value, "%s = %q has no place in the order", name, value)
	}
	require.Contains(t, statusLabels, statusUnprocessed)
}

// linkerConstants returns every string constant in go/citations whose name
// starts with prefix, keyed by constant name.
func linkerConstants(t *testing.T, prefix string) map[string]string {
	t.Helper()
	dir := filepath.Join("..", "go", "citations")
	pkgs, err := parser.ParseDir(token.NewFileSet(), dir, nil, 0)
	require.NoError(t, err)

	consts := make(map[string]string)
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.CONST {
					continue
				}
				for _, spec := range gen.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, ident := range vs.Names {
						if !strings.HasPrefix(ident.Name, prefix) || i >= len(vs.Values) {
							continue
						}
						lit, ok := vs.Values[i].(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							continue
						}
						value, err := strconv.Unquote(lit.Value)
						require.NoError(t, err)
						consts[ident.Name] = value
					}
				}
			}
		}
	}
	return consts
}

func TestChip(t *testing.T) {
	str := func(s string) *string { return &s }
	tests := []struct {
		name   string
		status *string
		tier   *string
		class  string
		label  string
		key    string
		linked bool
	}{
		{"unprocessed", nil, nil, "chip-skip", "Unprocessed", "unprocessed", false},
		{"linked cap", str(citations.StatusLinkedCAP), str(citations.TierCAPDirect), "chip-linked", "Linked (CAP)", "cap_direct", true},
		{"linked stub", str(citations.StatusLinkedStub), str(citations.TierStubDirect), "chip-linked", "Linked (stub case)", "stub_direct", true},
		{"no match", str(citations.StatusNoMatch), str(citations.TierUSPageAbsent), "chip-nomatch", "No match", "us_page_absent", false},
		{"junk", str(citations.StatusSkippedJunk), nil, "chip-junk", "Skipped as junk", "skipped_junk", false},
		{"statute", str(citations.StatusSkippedStatute), nil, "chip-junk", "Skipped as statute", "skipped_statute", false},
		{"not whitelisted", str(citations.StatusSkippedNotWhitelisted), nil, "chip-skip", "Not whitelisted", "skipped_not_whitelisted", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := chipFor(tt.status, tt.tier)
			require.Equal(t, tt.class, c.Class())
			require.Equal(t, tt.label, c.Label())
			require.Equal(t, tt.key, c.Key())
			require.Equal(t, tt.linked, c.Linked())
			require.NotEmpty(t, c.Gloss())
		})
	}
}
