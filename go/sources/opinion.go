package sources

import (
	"fmt"
	"strconv"
)

// CAPOpinion is one opinion of a case in the Caselaw Access Project: a row of
// cap.opinions, the unit cite-detector-cap detects citations in (issue #74),
// as a page of a treatise is the unit for cite-detector-moml. A case may have
// several opinions (a majority, dissents, concurrences, a rehearing), each
// with a text of its own; the opinion's own id keys the detections, because
// ("case", type) does not identify one, and its case is what the citation is
// counted against.
type CAPOpinion struct {
	OpinionID int64  // cap.opinions.id
	CaseID    int64  // cap.cases.id, the citing case
	Type      string // cap.opinions.type: majority, dissent, concurrence, rehearing, ...
	FullText  string
}

// NewCAPOpinion creates a CAP opinion document.
func NewCAPOpinion(opinionID, caseID int64, typ, text string) *CAPOpinion {
	return &CAPOpinion{
		OpinionID: opinionID,
		CaseID:    caseID,
		Type:      typ,
		FullText:  text,
	}
}

// String returns a string representation of the document.
func (o CAPOpinion) String() string {
	return fmt.Sprintf("Case <%d>, opinion <%d>", o.CaseID, o.OpinionID)
}

// ID returns the opinion's id as a string, as the Document interface asks.
// The store that saves the detections casts it back to bigint in SQL.
func (o *CAPOpinion) ID() string {
	return strconv.FormatInt(o.OpinionID, 10)
}

// ParentID returns the citing case's id as a string. Used to satisfy the
// Document interface.
func (o *CAPOpinion) ParentID() string {
	return strconv.FormatInt(o.CaseID, 10)
}

// HasParent returns true, because an opinion by definition belongs to a case.
// Used to satisfy the Document interface.
func (o *CAPOpinion) HasParent() bool {
	return true
}

// Text returns the full text of the opinion.
func (o *CAPOpinion) Text() string {
	return o.FullText
}

// CorrectOCR applies the OCR corrections to the opinion in a single pass. A nil
// replacer leaves the text alone.
func (o *CAPOpinion) CorrectOCR(r *OCRReplacer) {
	o.FullText = r.Replace(o.FullText)
}

// Rewrite replaces the text with f(text).
func (o *CAPOpinion) Rewrite(f func(string) string) {
	o.FullText = f(o.FullText)
}

// LogID returns the key-value pairs that identify the opinion in a log line.
func (o *CAPOpinion) LogID() []any {
	return []any{"cap_case", o.CaseID, "cap_opinion", o.OpinionID}
}
