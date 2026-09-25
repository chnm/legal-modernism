package sources

import "errors"

// ErrNotImplemented is for methods for which implementation does not make sense.
var ErrNotImplemented = errors.New("not implemented")

// ErrInvalidID means an improper ID has been passed to a query
var ErrInvalidID = errors.New("invalid ID(s)")

// ErrNoDocument means no document matches that query
var ErrNoDocument = errors.New("no matching document")
