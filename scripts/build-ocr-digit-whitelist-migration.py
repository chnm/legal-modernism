#!/usr/bin/env python3
"""Build the whitelist migration from the OCR digit candidate list.

Two filters, per the review decision:

  1. canonical only -- the stripped spelling is itself a reporter in
     legalhist.reporters, not another whitelist variant that maps onward.

  2. unambiguous only -- stripInteriorDigits assumes the digit was INSERTED into
     the spelling ("Fed." scanned as "F1ed."). The OCR can equally well have
     SUBSTITUTED a digit for a letter ("Mic." scanned as "M1c."), which points at
     a different reporter. A candidate is ambiguous when replacing the digit with
     some letter yields a whitelist entry resolving to a different reporter than
     the strip does; those are dropped rather than guessed at.
"""
import ast
import string
import sys
from collections import OrderedDict

CANDIDATES, WHITELIST, COUNTS, OUT = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]

# An alternate reading only counts against a candidate if it is plausible, and
# corpus frequency is the evidence for that. "M1o." could in principle be "Moo."
# (Mood's reports), but "Mo." outnumbers "Moo." 37 to 1, so the reading is not in
# real doubt. "M1d." is a different matter: "Mod." is 71% as common as "Md."
ALT_THRESHOLD = 0.10

spelling_counts = {}
with open(COUNTS, encoding="utf-8") as fh:
    for line in fh:
        parts = line.rstrip("\n").split("\t")
        if len(parts) == 2 and parts[1].isdigit():
            spelling_counts[parts[0]] = int(parts[1])

whitelist = {}
with open(WHITELIST, encoding="utf-8") as fh:
    for line in fh:
        parts = line.rstrip("\n").split("\t")
        if len(parts) != 3:
            continue
        found, standard, junk = parts
        whitelist[found] = (standard, junk == "t")

rows = []
with open(CANDIDATES, encoding="utf-8") as fh:
    for line in fh:
        if line.startswith("#") or not line.strip():
            continue
        count, found, stripped, standard, conf, example = line.rstrip("\n").split("\t")
        rows.append({
            "count": int(count),
            "found": ast.literal_eval(found),
            "stripped": ast.literal_eval(stripped),
            "standard": ast.literal_eval(standard),
            "conf": conf,
            "example": ast.literal_eval(example),
        })

canonical = [r for r in rows if r["conf"] == "canonical"]


def alternate_readings(found, stripped, strip_standard):
    """Plausible reporters reachable by reading the digit as a letter.

    Returns {reporter: (spelling, count, share)} for alternates that occur often
    enough in moml_citations.citations_unlinked to put the strip reading in
    doubt. Rarer alternates are real words but not real competition.
    """
    base = max(spelling_counts.get(stripped, 0), 1)
    alts = {}
    for i, ch in enumerate(found):
        if not ch.isdigit():
            continue
        for letter in string.ascii_letters:
            cand = found[:i] + letter + found[i + 1:]
            entry = whitelist.get(cand)
            if not (entry and not entry[1] and entry[0] and entry[0] != strip_standard):
                continue
            n = spelling_counts.get(cand, 0)
            if n / base >= ALT_THRESHOLD:
                alts[entry[0]] = (cand, n, n / base)
    return alts


keep, dropped = [], []
for r in canonical:
    alts = alternate_readings(r["found"], r["stripped"], r["standard"])
    if alts:
        r["alts"] = alts
        dropped.append(r)
    else:
        keep.append(r)

keep.sort(key=lambda r: (-r["count"], r["found"]))
dropped.sort(key=lambda r: (-r["count"], r["found"]))

# Collapse to one row per spelling; the same spelling cannot appear twice
# because reporter_found is the primary key.
seen = OrderedDict()
for r in keep:
    seen.setdefault(r["found"], r)
keep = list(seen.values())

sql = []
w = sql.append
w("-- migrate:up")
w("SET ROLE = law_admin;")
w("")
w("-- Seed legalhist.whitelist with reporter abbreviations the OCR corrupted by")
w("-- reading a letter as a digit: \"Fed.\" scanned as \"F1ed.\", \"Mass.\" as")
w("-- \"M1ass.\". GenericOCRDigitDetector finds these citations, and like every")
w("-- other detector it records the spelling as it appeared, so they sit at")
w("-- skipped_not_whitelisted until the whitelist carries the spelling.")
w("--")
w("-- Candidates were compiled by TestOCRDigitWhitelistSuggestions over a")
w("-- 1,052,158-page sample (10% of moml.page_ocrtext) and are recorded in full,")
w("-- including the ones rejected here, in db/whitelist-candidates-ocr-digits.tsv.")
w("--")
w("-- Two filters were applied to the %d proposals in that file:" % len(rows))
w("--")
w("--   1. Canonical only. The spelling with its digits removed must itself be a")
w("--      reporter in legalhist.reporters, not another whitelist variant that")
w("--      maps onward. This drops proposals that would inherit an existing")
w("--      entry's looseness, e.g. \"E1q.\" -> \"Eq.\" -> C.L.R.")
w("--")
w("--   2. Unambiguous only. Removing the digit assumes the OCR INSERTED it. The")
w("--      OCR can equally well have SUBSTITUTED a digit for a letter, which")
w("--      points at a different reporter, so a candidate is dropped when some")
w("--      letter in place of the digit yields a whitelist entry for a different")
w("--      reporter that is common enough in the corpus to be real competition")
w("--      (at least %d%% as frequent as the strip reading in" % int(ALT_THRESHOLD * 100))
w("--      moml_citations.citations_unlinked). Rare alternates do not count:")
w("--      \"M1o.\" could be \"Moo.\" (Mood) in principle, but \"Mo.\" outnumbers it")
w("--      37 to 1. %d canonical proposals were dropped this way:" % len(dropped))
for r in dropped:
    alt_desc = "; ".join(
        '"%s" -> %s at %.0f%% of its frequency' % (v[0], k, 100 * v[2])
        for k, v in sorted(r["alts"].items(), key=lambda kv: -kv[1][2]))
    w('--        "%s" -> %s, but also reads as %s' % (r["found"], r["standard"], alt_desc))
w("--")
w("-- %d rows remain. Counts below are occurrences in the 10%% sample, so the" % len(keep))
w("-- corpus-wide figure is roughly ten times each.")
w("")
w("INSERT INTO legalhist.whitelist (reporter_found, reporter_standard, junk) VALUES")
for i, r in enumerate(keep):
    end = "," if i < len(keep) - 1 else ""
    w("    ('%s', '%s', false)%s  -- %d, e.g. %s" % (
        r["found"].replace("'", "''"),
        r["standard"].replace("'", "''"),
        end, r["count"], r["example"].replace("\n", " ").replace("\t", " ").strip()))
w("ON CONFLICT (reporter_found) DO NOTHING;")
w("")
w("-- migrate:down")
w("SET ROLE = law_admin;")
w("")
w("DELETE FROM legalhist.whitelist WHERE reporter_found IN (")
for i, r in enumerate(keep):
    end = "," if i < len(keep) - 1 else ""
    w("    '%s'%s" % (r["found"].replace("'", "''"), end))
w(");")
w("")

with open(OUT, "w", encoding="utf-8") as fh:
    fh.write("\n".join(sql))

print("proposals in file:      %d" % len(rows))
print("canonical:              %d" % len(canonical))
print("dropped as ambiguous:   %d" % len(dropped))
print("SEEDED:                 %d spellings, %d sample rows" % (len(keep), sum(r["count"] for r in keep)))
print()
for r in dropped:
    print("  DROPPED %-12r -> %-10s also reads as %s" % (
        r["found"], r["standard"],
        "; ".join('%r -> %s (%.0f%%)' % (v[0], k, 100 * v[2])
                  for k, v in sorted(r["alts"].items(), key=lambda kv: -kv[1][2]))))
