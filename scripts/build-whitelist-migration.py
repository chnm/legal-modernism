#!/usr/bin/env python3
"""Build a legalhist.whitelist seed migration from an OCR-variant candidate file.

Reads the TSV written by TestOCRVariantWhitelistSuggestions
(go/citations/ocrvariant_whitelist_test.go) and emits a dbmate migration that
inserts the proposals surviving two filters, the ones PR #241 applied by hand:

  1. canonical only -- the corrected spelling is itself a reporter in
     legalhist.reporters, not another whitelist variant that maps onward.

  2. unambiguous only -- the harness already marks a spelling AMBIGUOUS when a
     competing reading reaches a different reporter at 10% or more of the
     winner's corpus frequency, and those rows are in a separate section of
     the file, so they never reach this script. Proposals that exist only by
     reading a digit as an arbitrary letter (rule digit_letter) are review
     material, not evidence, and are dropped here too.

  3. long enough for a letter confusion -- a digit inside an abbreviation is
     always an OCR error, so the digit rules stand on their own, but a
     letter-for-letter reading of a short abbreviation is as plausible a word
     as its input: the first run proposed "Out." as Ontario when it is
     Outerbridge's Pennsylvania Reports, and "Gal." as California when it is
     as likely Gallison. A confusion reading is seeded only when its corrected
     spelling has at least MIN_CONFUSION_LETTERS letters; shorter ones are
     listed in the header for review.

Usage: build-whitelist-migration.py CANDIDATES.tsv OUT.sql [DESCRIPTION]

DESCRIPTION is a short phrase for the header, e.g. the harness's source line.
The generalization of scripts/build-ocr-digit-whitelist-migration.py, which is
kept unchanged as the record of the #241 batch.
"""
import ast
import sys
from collections import OrderedDict

CANDIDATES, OUT = sys.argv[1], sys.argv[2]
DESCRIPTION = sys.argv[3] if len(sys.argv) > 3 else ""
SEEDABLE_RULES = {"digit", "digit_sub", "confusion"}
MIN_CONFUSION_LETTERS = 5


def letters(s):
    return sum(1 for ch in s if ch.isalpha())


def long_enough(r):
    return r["rule"] != "confusion" or letters(r["corrected"]) >= MIN_CONFUSION_LETTERS

rows, ambiguous, tojunk, unresolved = [], 0, 0, 0
source = ""
with open(CANDIDATES, encoding="utf-8") as fh:
    for line in fh:
        line = line.rstrip("\n")
        if line.startswith("# source:"):
            source = line[len("# source:"):].strip()
        if line.startswith("#AMBIGUOUS\t"):
            ambiguous += 1
        elif line.startswith("#TOJUNK\t"):
            tojunk += 1
        elif line.startswith("#UNRESOLVED\t"):
            unresolved += 1
        if line.startswith("#") or not line.strip():
            continue
        count, found, corrected, standard, rule, conf, example = line.split("\t")
        rows.append({
            "count": int(count),
            "found": ast.literal_eval(found),
            "corrected": ast.literal_eval(corrected),
            "standard": ast.literal_eval(standard),
            "rule": rule,
            "conf": conf,
            "example": ast.literal_eval(example),
        })

canonical = [r for r in rows if r["conf"] == "canonical"]
seedable_rule = [r for r in canonical if r["rule"] in SEEDABLE_RULES]
keep = [r for r in seedable_rule if long_enough(r)]
short = [r for r in seedable_rule if not long_enough(r)]
short.sort(key=lambda r: (-r["count"], r["found"]))
dropped_variant = len(rows) - len(canonical)
dropped_digit_letter = len(canonical) - len(seedable_rule)

keep.sort(key=lambda r: (-r["count"], r["found"]))
seen = OrderedDict()
for r in keep:
    seen.setdefault(r["found"], r)
keep = list(seen.values())
by_rule = {}
for r in keep:
    by_rule.setdefault(r["rule"], [0, 0])
    by_rule[r["rule"]][0] += 1
    by_rule[r["rule"]][1] += r["count"]


def q(s):
    return s.replace("'", "''")


def clean(s):
    return s.replace("\n", " ").replace("\t", " ").strip()


sql = []
w = sql.append
w("-- migrate:up")
w("SET ROLE = law_admin;")
w("")
w("-- Seed legalhist.whitelist with reporter abbreviations the OCR corrupted by")
w("-- confusing one letter for another (\"Vill.\" for \"Will.\", \"Fcd.\" for \"Fed.\",")
w("-- \"Cornst.\" for \"Comst.\") or by reading a letter as a digit (\"I1l.\" for")
w("-- \"Ill.\"). Every detector records the spelling as it appeared, so these")
w("-- citations sit at skipped_not_whitelisted until the whitelist carries the")
w("-- spelling (issue #247, part of #165).")
w("--")
w("-- Candidates were compiled by TestOCRVariantWhitelistSuggestions from")
w("-- %s," % (source or DESCRIPTION or "moml_citations.citations_unlinked"))
w("-- and are recorded in full, including the ones rejected here, in")
w("-- %s." % CANDIDATES)
w("--")
w("-- A candidate is one non-whitelisted spelling with a reading that is already")
w("-- an exact whitelist entry; readings come from stripping a letter-flanked")
w("-- digit or from one substitution in the OCR confusion table of")
w("-- go/citations/ocrvariant.go. Nothing is normalized. Two filters were then")
w("-- applied to the %d proposals in that file:" % len(rows))
w("--")
w("--   1. Canonical only. The corrected spelling must itself be a reporter in")
w("--      legalhist.reporters, not another whitelist variant that maps onward,")
w("--      which would inherit that entry's looseness. %d proposals dropped." % dropped_variant)
w("--")
w("--   2. Unambiguous only. The harness compares every reporter a spelling's")
w("--      readings reach by the corpus frequency of the spellings that reach")
w("--      them, and sets aside any spelling whose runner-up is at least 10%% as")
w("--      frequent as its winner; %d such spellings are in the file's AMBIGUOUS" % ambiguous)
w("--      section and none is seeded. A reading reached only by taking a digit")
w("--      for an arbitrary letter (rule digit_letter) is the check's evidence,")
w("--      not a proposal: %d canonical proposals of that kind were dropped." % dropped_digit_letter)
w("--")
w("--   3. Long enough. A letter-for-letter reading is seeded only when the")
w("--      corrected spelling has at least %d letters, because on a shorter" % MIN_CONFUSION_LETTERS)
w("--      abbreviation the reading is as plausible a word as the scanned one")
w("--      (\"Out.\" is Outerbridge, not Ontario; \"Gal.\" is as likely Gallison as")
w("--      California). Digit readings are exempt: a digit inside an abbreviation")
w("--      is always an OCR error. %d canonical proposals were set aside for" % len(short))
w("--      review this way, %s rows in all:" % format(sum(r["count"] for r in short), ","))
for r in short:
    w("--        %-14s -> %-14s %-12s %6d rows" % ('"%s"' % r["found"], r["standard"], "(" + r["corrected"] + ")", r["count"]))
w("--")
w("-- %d rows remain, by rule:" % len(keep))
for rule in sorted(by_rule):
    w("--   %-10s %5d spellings %10s rows" % (rule, by_rule[rule][0], format(by_rule[rule][1], ",")))
w("-- The file also lists %d spellings whose every reading is junk (TOJUNK) and" % tojunk)
w("-- %d with no whitelisted reading (UNRESOLVED); neither is touched here." % unresolved)
w("-- Counts below are rows in moml_citations.citations_unlinked.")
w("")
w("INSERT INTO legalhist.whitelist (reporter_found, reporter_standard, junk) VALUES")
for i, r in enumerate(keep):
    end = "," if i < len(keep) - 1 else ""
    w("    ('%s', '%s', false)%s  -- %d, %s, e.g. %s" % (
        q(r["found"]), q(r["standard"]), end, r["count"], r["rule"], clean(r["example"])))
w("ON CONFLICT (reporter_found) DO NOTHING;")
w("")
w("-- migrate:down")
w("SET ROLE = law_admin;")
w("")
w("DELETE FROM legalhist.whitelist WHERE reporter_found IN (")
for i, r in enumerate(keep):
    end = "," if i < len(keep) - 1 else ""
    w("    '%s'%s" % (q(r["found"]), end))
w(");")
w("")

with open(OUT, "w", encoding="utf-8") as fh:
    fh.write("\n".join(sql))

print("proposals in file:        %d" % len(rows))
print("canonical:                %d" % len(canonical))
print("dropped as digit_letter:  %d" % dropped_digit_letter)
print("set aside as short:       %d spellings, %d rows" % (len(short), sum(r["count"] for r in short)))
print("ambiguous (in file):      %d" % ambiguous)
print("SEEDED:                   %d spellings, %d rows" % (len(keep), sum(r["count"] for r in keep)))
