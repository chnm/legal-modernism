package citations

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMatchTierConstantsAreAllowedBySQL guards the one way the tier column can
// fail in production: chk_citation_links_match_tier rejects a value the linker
// emits, which aborts the whole insert batch rather than one row. The tier names
// live in two places by necessity — Go writes them, SQL constrains them — so this
// reads every Tier* constant straight out of the source (no hand-maintained list
// to drift) and requires each one to appear in some migration.
func TestMatchTierConstantsAreAllowedBySQL(t *testing.T) {
	tiers := tierConstants(t)
	require.NotEmpty(t, tiers, "found no Tier* constants; has the naming changed?")

	migrations, err := filepath.Glob(filepath.Join("..", "..", "db", "migrations", "*.sql"))
	require.NoError(t, err)
	require.NotEmpty(t, migrations, "found no migrations to check against")

	// Every migration is searched, not just the one that adds the constraint, so
	// that widening it later in its own migration also satisfies this.
	var sql strings.Builder
	for _, m := range migrations {
		b, err := os.ReadFile(m)
		require.NoError(t, err)
		sql.Write(b)
	}
	all := sql.String()

	for name, value := range tiers {
		require.Contains(t, all, "'"+value+"'",
			"%s = %q is not allowed by any migration; widen chk_citation_links_match_tier", name, value)
	}
}

// tierConstants returns every Tier*-named string constant declared in this
// package, keyed by constant name.
func tierConstants(t *testing.T) map[string]string {
	t.Helper()

	pkgs, err := parser.ParseDir(token.NewFileSet(), ".", nil, 0)
	require.NoError(t, err)

	tiers := make(map[string]string)
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
						if !strings.HasPrefix(ident.Name, "Tier") || i >= len(vs.Values) {
							continue
						}
						lit, ok := vs.Values[i].(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							continue
						}
						value, err := strconv.Unquote(lit.Value)
						require.NoError(t, err)
						tiers[ident.Name] = value
					}
				}
			}
		}
	}
	return tiers
}
