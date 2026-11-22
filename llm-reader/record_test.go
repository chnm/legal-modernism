package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRecordCheck(t *testing.T) {
	id := int64(7)
	base := func(cs ...Citation) PageRecord {
		return PageRecord{Psmid: "v1", Pageid: "00010", Model: "m", ReadAt: time.Now(), Citations: append([]Citation{}, cs...)}
	}
	tests := []struct {
		name string
		rec  PageRecord
		path string
		errs int
	}{
		{"empty page", base(), "results/v1/00010.json", 0},
		{"nil citations", PageRecord{Psmid: "v1", Pageid: "00010", Model: "m", ReadAt: time.Now()}, "results/v1/00010.json", 1},
		{"wrong file name", base(), "results/v2/00010.json", 1},
		{"linked cap", base(Citation{Seq: 1, Raw: "1 Johns. 1", Status: "linked_cap", CapCaseID: &id, Confidence: "high"}), "results/v1/00010.json", 0},
		{"linked with wrong target", base(Citation{Seq: 1, Raw: "x", Status: "linked_cap", ErCaseID: "1", Confidence: "high"}), "results/v1/00010.json", 1},
		{"no_match with target", base(Citation{Seq: 1, Raw: "x", Status: "no_match", StubCite: "1 W.R. 1", Confidence: "low"}), "results/v1/00010.json", 1},
		{"bad seq, status, confidence", base(Citation{Seq: 2, Raw: "x", Status: "linked", Confidence: "sure"}), "results/v1/00010.json", 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Len(t, tt.rec.check(tt.path), tt.errs)
		})
	}
}
