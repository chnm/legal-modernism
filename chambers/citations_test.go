package main

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCitationFilterShape(t *testing.T) {
	tests := []struct {
		query string
		shape string
	}{
		{"", ""},
		{"status=no_match", ""},
		{"cite=2+Mass.+420", "cite"},
		{"reporter=Mass.&volume=2&page=420", "cite"},
		{"reporter=Cranch&page=137", "cite"},
		{"reporter=Mass.", "reporter"},
		{"reporter=Mass.&volume=2", "reporter"},
		{"edition=CTRG95-B2993", "edition"},
		{"edition=CTRG95-B2993&case=cap:1", "edition"},
		{"case=cap:1", "case"},
		{"case=cap:1&reporter=Mass.", "case"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			q, err := url.ParseQuery(tt.query)
			require.NoError(t, err)
			require.Equal(t, tt.shape, parseCitationFilter(q).Shape())
		})
	}

	f := parseCitationFilter(url.Values{"cite": {" 2 Mass. 420 "}, "tier": {"cap_direct"}})
	require.Equal(t, "2 Mass. 420", f.Cite)
	require.Equal(t, "cite=2+Mass.+420&tier=cap_direct", f.Values().Encode())
}

func TestCitationQueryAddCase(t *testing.T) {
	q := &citationQuery{}
	q.arg("first")
	require.NoError(t, q.addCase("cap:6754004"))
	require.Equal(t, []string{"cl.cap_case_id = $2::bigint"}, q.where)
	require.Equal(t, []any{"first", "6754004"}, q.args)

	q = &citationQuery{}
	require.NoError(t, q.addCase("stub:[1905] 2 K.B. 1"))
	require.Equal(t, []string{"cl.stub_cite = $1"}, q.where)
	require.Error(t, q.addCase("bogus"))
}

func TestLooksLikeCite(t *testing.T) {
	for _, s := range []string{"2 Mass. 420", "Cranch 137", "[1905] 2 K.B. 1", "7 Ad. & El. 540", "1 N.C. (Taylor) 1", "145 E.R. 696"} {
		require.True(t, looksLikeCite(s), s)
	}
	for _, s := range []string{"Marbury v. Madison", "Bradberry", "", "2 420"} {
		require.False(t, looksLikeCite(s), s)
	}
}

func TestNullableInt(t *testing.T) {
	require.Nil(t, nullableInt(""))
	require.Nil(t, nullableInt("x"))
	require.Equal(t, 4, *nullableInt("4"))
}
