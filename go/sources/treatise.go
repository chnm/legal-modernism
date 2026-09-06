package sources

import "fmt"

// TreatisePage represents a page from a treatise in MOML.
type TreatisePage struct {
	PageID     string
	TreatiseID string
	FullText   string
}

// String returns a string representation of the document.
func (t TreatisePage) String() string {
	return fmt.Sprintf("Treatise <%s>, page <%s>", t.TreatiseID, t.PageID)
}

// ID returns the ID of the page from the treatise.
func (t *TreatisePage) ID() string {
	return t.PageID
}

// Text returns the full text of the page of the treatise.
func (t *TreatisePage) Text() string {
	return t.FullText
}

// ParentID returns the ID of the treatise. Used to satisfy the Document interface.
func (t *TreatisePage) ParentID() string {
	return t.TreatiseID
}

// HasParent returns true, because a page from a treatise by definition has a
// parent document. Used to satisfy the Document interface.
func (t *TreatisePage) HasParent() bool {
	return true
}

// CorrectOCR applies the OCR corrections to the page in a single pass. A nil
// replacer leaves the text alone.
func (t *TreatisePage) CorrectOCR(r *OCRReplacer) {
	t.FullText = r.Replace(t.FullText)
}

// NewTreatisePage creates a new treatise document
func NewTreatisePage(pageID string, treatiseID, text string) *TreatisePage {
	return &TreatisePage{
		PageID:     pageID,
		TreatiseID: treatiseID,
		FullText:   text,
	}
}
