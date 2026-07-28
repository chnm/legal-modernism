package citations

import "regexp"

var reVolume = regexp.MustCompile(`^\d+`)
var rePage = regexp.MustCompile(`\d+$`)

// var reAbbr = regexp.MustCompile(`\s*[\w\.]+\s*`)

// Multiple spaces in a row
var reSpace = regexp.MustCompile(`\s+`)

// Multiple periods in a row
var reMultiplePeriodsSpace = regexp.MustCompile(`\.+\s`)
var reMultiplePeriodsNoSpace = regexp.MustCompile(`\.+`)
