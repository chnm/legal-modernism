package sources

// Document models a document
type Document interface {
	ID() string
	ParentID() string
	HasParent() bool
	Text() string
	CorrectOCR(*OCRReplacer)
	// Pages() []*Document
	// HasPages() bool
}

// Doc is a simple document which has no pages, parent, or other subdivisions.
type Doc struct {
	Identifier string
	FullText   string
}

// String returns a string representation of the document.
func (d Doc) String() string {
	return d.Identifier
}

// ID returns the ID of the document.
func (d *Doc) ID() string {
	return d.Identifier
}

// Text returns the full text of the document.
func (d *Doc) Text() string {
	return d.FullText
}

// ParentID returns an empty string, because a Doc by definition has no parent
// document. Used to satisfy the Document interface.
func (d *Doc) ParentID() string {
	return ""
}

// HasParent returns false, because a Doc by definition has not parent document.
// Used to satisfy the Document interface.
func (d *Doc) HasParent() bool {
	return false
}

// CorrectOCR applies the OCR corrections to the document in a single pass. A nil
// replacer leaves the text alone.
func (d *Doc) CorrectOCR(r *OCRReplacer) {
	d.FullText = r.Replace(d.FullText)
}

// Pages returns an empty slice of Documents, because a Doc by definition has no
// parent document. Used to satisfy the Document interface.
// func (d *Doc) Pages() []*Document {
// 	pages := make([]*Document, 0)
// 	return pages
// }

// HasPages returns false, because a Doc by definition has no parent document.
// Used to satisfy the Document interface.
// func (d *Doc) HasPages() bool {
// 	return false
// }

// NewDoc creates a new document
func NewDoc(id string, text string) *Doc {
	return &Doc{
		Identifier: id,
		FullText:   text,
	}
}
