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

// A digit with a letter on either side. In this corpus that is always an OCR
// misreading of a letter rather than a real part of an abbreviation: "Fed."
// scanned as "F1ed.", "Mass." as "Ma5ss.". The digits that legitimately appear
// in a reporter abbreviation are series designators ("Wn. (2d)", "A.S.R.3d")
// and edition numbers ("Leach, 4th ed."), where the digit follows a space or a
// period rather than a letter, so this pattern leaves them alone. No row in
// legalhist.reporters, legalhist.reporters_abbreviations, or
// legalhist.whitelist matches it.
var reInteriorDigit = regexp.MustCompile(`(\p{L})\d(\p{L})`)
