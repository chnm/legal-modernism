package main

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNum(t *testing.T) {
	n := 1234567
	var nilInt *int
	require.Equal(t, "0", num(0))
	require.Equal(t, "999", num(999))
	require.Equal(t, "1,000", num(1000))
	require.Equal(t, "1,234,567", num(n))
	require.Equal(t, "1,234,567", num(&n))
	require.Equal(t, "-1,234", num(int64(-1234)))
	require.Equal(t, "—", num(nilInt))
	require.Equal(t, "3", num(2.6))
}

func TestPct(t *testing.T) {
	require.Equal(t, "—", pct(1, 0))
	require.Equal(t, "50%", pct(1, 2))
	require.Equal(t, "57%", pct(31806455, 56032703))
	require.Equal(t, "100%", pct(3, 3))
}

func TestYearSpan(t *testing.T) {
	a, b := 1765, 1854
	require.Equal(t, "1765–1854", yearSpan(&a, &b))
	require.Equal(t, "1765", yearSpan(&a, &a))
	require.Equal(t, "1765", yearSpan(&a, nil))
	require.Equal(t, "—", yearSpan(nil, nil))
}

func TestCleanRawAndTruncate(t *testing.T) {
	require.Equal(t, "2 Mass. 420", cleanRaw("2 Mass.\n\t\t\t420"))
	require.Equal(t, "abcde", truncate("abcde", 5))
	require.Equal(t, "abcd…", truncate("abcdef", 4))
}

func TestPaginate(t *testing.T) {
	r := httptest.NewRequest("GET", "/works?q=law&page=2", nil)

	p := paginate(r, 2, 100, 100, 350)
	require.True(t, p.HasPrev)
	require.True(t, p.HasNext)
	require.Equal(t, "/works?page=1&q=law", p.PrevURL)
	require.Equal(t, "/works?page=3&q=law", p.NextURL)
	require.Equal(t, 101, p.From())
	require.Equal(t, 200, p.To())

	p = paginate(r, 4, 100, 50, 350)
	require.False(t, p.HasNext, "the last page of a counted list has no next")
	require.Equal(t, 350, p.To())

	p = paginate(r, 1, 100, 100, -1)
	require.True(t, p.HasNext, "a full page of an uncounted list may have more")
	require.False(t, p.HasPrev)
	p = paginate(r, 1, 100, 99, -1)
	require.False(t, p.HasNext)
	require.Equal(t, 1, p.From())
	require.Equal(t, 0, paginate(r, 1, 100, 0, 0).From())

	require.Equal(t, 1, parsePage(""))
	require.Equal(t, 1, parsePage("0"))
	require.Equal(t, 1, parsePage("x"))
	require.Equal(t, 7, parsePage("7"))
}
