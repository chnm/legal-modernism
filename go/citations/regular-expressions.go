package citations

import "regexp"

var reVolume = regexp.MustCompile(`^\d+`)
var rePage = regexp.MustCompile(`\d+$`)

// var reAbbr = regexp.MustCompile(`\s*[\w\.]+\s*`)

// Multiple spaces in a row
var reSpace = regexp.MustCompile(`\s+`)

// Any letter. abbrChar admits whitespace, periods, commas, ampersands and
// parentheses as well as letters, so a match can consist entirely of the leader
// dots between two numbers in a table of contents; reHasLetter is how Detect
// rejects those (issue #283).
var reHasLetter = regexp.MustCompile(`\p{L}`)

// Multiple periods in a row
var reMultiplePeriodsSpace = regexp.MustCompile(`\.+\s`)
var reMultiplePeriodsNoSpace = regexp.MustCompile(`\.+`)
