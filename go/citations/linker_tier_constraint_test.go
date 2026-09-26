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
//
// Since issue #74 the CHECK exists twice, on moml_citations.citation_links and
// on opinion_citations.citation_links, one per corpus, and the two are kept in
// step by hand: one cascade writes both tables, so a tier must be admitted by
// both. Each tier is therefore required in a migration that names each table,
// so that widening one CHECK and forgetting the other cannot pass. The test is
// a text grep, not a parse of the constraint: a migration that names a table
// for another reason and quotes a tier — a materialized view over
// citation_links that filters on 'cap_page_interior', say — satisfies it for
// that tier too. It catches the forgotten migration, not a mistyped one.
func TestMatchTierConstantsAreAllowedBySQL(t *testing.T) {
	tiers := stringConstants(t, "Tier")
	require.NotEmpty(t, tiers, "found no Tier* constants; has the naming changed?")

	migrations, err := filepath.Glob(filepath.Join("..", "..", "db", "migrations", "*.sql"))
	require.NoError(t, err)
	require.NotEmpty(t, migrations, "found no migrations to check against")

	// Every migration is searched, not just the one that adds the constraint, so
	// that widening it later in its own migration also satisfies this. A
	// migration's text counts towards each of the tables it names.
	tables := []string{"moml_citations.citation_links", "opinion_citations.citation_links"}
	sql := make(map[string]*strings.Builder, len(tables))
	for _, table := range tables {
		sql[table] = &strings.Builder{}
	}
	for _, m := range migrations {
		b, err := os.ReadFile(m)
		require.NoError(t, err)
		for _, table := range tables {
			if strings.Contains(string(b), table) {
				sql[table].Write(b)
			}
		}
	}

	for _, table := range tables {
		all := sql[table].String()
		require.NotEmpty(t, all, "no migration names %s", table)
		for name, value := range tiers {
			require.Contains(t, all, "'"+value+"'",
				"%s = %q is not allowed by any migration naming %s; widen its chk_citation_links_match_tier", name, value, table)
		}
	}
}

// stringConstants returns every string constant declared in this package whose
// name starts with prefix ("Tier", "Status"), keyed by constant name.
func stringConstants(t *testing.T, prefix string) map[string]string {
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
						if !strings.HasPrefix(ident.Name, prefix) || i >= len(vs.Values) {
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
