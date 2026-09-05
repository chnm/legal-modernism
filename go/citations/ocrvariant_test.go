package citations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfusionVariants(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []string // must be present
		absent  []string // must not be present
		selfOut bool
	}{
		{"V for W", "Vill.", []string{"Will."}, nil, true},
		{"c for e", "Fcd.", []string{"Fed."}, nil, true},
		{"rn for m", "Cornst.", []string{"Comst."}, nil, true},
		{"digit for letter", "I1l.", []string{"Ill.", "Il1."}, nil, true},
		{"two sites are separate variants", "Vv.", []string{"Wv.", "Vw."}, []string{"Ww."}, true},
		{"multi-character confusion", "vv.", []string{"w."}, nil, true},
		{"no confusable characters", "Mass.", nil, []string{"Mass."}, true},
		{"empty", "", nil, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := confusionVariants(tt.in)
			for _, w := range tt.want {
				assert.Contains(t, got, w)
			}
			for _, a := range tt.absent {
				assert.NotContains(t, got, a)
			}
			assert.NotContains(t, got, tt.in, "the input is never its own variant")
			seen := map[string]bool{}
			for _, g := range got {
				assert.False(t, seen[g], "duplicate variant %q", g)
				seen[g] = true
			}
		})
	}
}

func TestReadingsRuleOrder(t *testing.T) {
	got := readings("M1o.")
	require.NotEmpty(t, got)
	assert.Equal(t, Reading{Corrected: "Mo.", Rule: RuleDigit}, got[0])
	// "1" -> "l" and "1" -> "I" are confusion readings of the same spelling,
	// and keep that rule even though the digit-as-letter pass reaches them too.
	assert.Contains(t, got, Reading{Corrected: "Mlo.", Rule: RuleConfusion})
	assert.Contains(t, got, Reading{Corrected: "MIo.", Rule: RuleConfusion})
	// Any other letter in the digit's place is a review-only reading.
	assert.Contains(t, got, Reading{Corrected: "Moo.", Rule: RuleDigitLetter})
	assert.NotContains(t, got, Reading{Corrected: "Moo.", Rule: RuleConfusion})
	// A digit that is not letter-flanked is left alone by that pass.
	for _, r := range readings("Wn. (2d)") {
		assert.NotEqual(t, RuleDigitLetter, r.Rule)
	}
}

func TestResolve(t *testing.T) {
	lookup := &WhitelistLookup{
		Standard: map[string]string{
			"Will.": "Wil.", "Ill.": "Ill.", "Il.": "Ill.", "Mo.": "Mo.", "Moo.": "Moo",
			"Md.": "Md.", "Mod.": "Mod", "Fed.": "Fed.", "Feb.": "",
		},
		Junk:      map[string]bool{"Feb.": true},
		Canonical: map[string]bool{"Ill.": true, "Mo.": true, "Md.": true, "Fed.": true, "Wil.": true},
		Freq:      map[string]int{"Will.": 500, "Ill.": 9000, "Il.": 40, "Mo.": 3700, "Moo.": 100, "Md.": 1000, "Mod.": 710, "Fed.": 5000},
	}
	tests := []struct {
		name      string
		found     string
		wantKind  string
		wantBest  string
		wantRule  string
		wantCanon bool
	}{
		{"single reporter, canonical", "M1o.", KindProposed, "Mo.", RuleDigit, true},
		// "M1o." also reads as "Moo." (Mood) at 100 against 3700: no competition.
		{"rare alternate is not competition", "M1o.", KindProposed, "Mo.", RuleDigit, true},
		// "M1d." reads as "Md." by the strip and as "Mod." by the digit-as-letter
		// pass, and Mod. is 71% as frequent as Md.
		{"competing reporter above 10% is ambiguous", "M1d.", KindAmbiguous, "", "", false},
		{"confusion reading", "Vill.", KindProposed, "Will.", RuleConfusion, false},
		{"canonical spelling beats the variant of the same reporter", "I1l.", KindProposed, "Ill.", RuleConfusion, true},
		{"junk only", "Fcb.", KindToJunk, "", "", false},
		{"reporter beats junk", "Fcd.", KindProposed, "Fed.", RuleConfusion, true},
		{"nothing known", "Xyzzy.", KindUnresolved, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolve(tt.found, lookup, 0.10)
			assert.Equal(t, tt.wantKind, got.Kind)
			if tt.wantKind == KindProposed {
				assert.Equal(t, tt.wantBest, got.Best.Corrected)
				assert.Equal(t, tt.wantRule, got.Best.Rule)
				assert.Equal(t, tt.wantCanon, got.Best.Canonical)
			}
		})
	}

	t.Run("threshold boundary", func(t *testing.T) {
		// Md. at 1000 against Mod. at 710: ambiguous at 10%, proposed at 75%.
		assert.Equal(t, KindAmbiguous, resolve("M1d.", lookup, 0.10).Kind)
		assert.Equal(t, KindProposed, resolve("M1d.", lookup, 0.75).Kind)
	})
}
