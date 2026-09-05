#!/usr/bin/env python3
"""Build the junk-and-statute seed migration from an OCR-variant candidate file.

The TOJUNK section of the file written by TestOCRVariantWhitelistSuggestions
(go/citations/ocrvariant_whitelist_test.go) holds the non-whitelisted spellings
whose every reading lands on a junk whitelist entry: "Cye." for the junk
"Cyc.", "Dee." for "Dec.", "ii" for "n". Such a spelling is OCR damage to
something the whitelist already rejects, so it gets a junk row of its own.

One exception: many junk entries are regnal-year statute spellings that
20260905120000_seed-statute-reporters.sql reroutes to the statute rows ("Gco.
IV. c." reads as "Geo. IV. c.", which is now Stat. Geo.). A reading of one of
those is seeded to the same statute row, not to junk, so the two migrations
agree. The statute migration is read to find which spellings those are.

Usage: build-whitelist-junk-migration.py CANDIDATES.tsv STATUTE_MIGRATION.sql OUT.sql
"""
import ast
import re
import sys

CANDIDATES, STATUTE_MIGRATION, OUT = sys.argv[1], sys.argv[2], sys.argv[3]

statute_text = open(STATUTE_MIGRATION, encoding="utf-8").read()
block = statute_text.split("FROM (VALUES", 2)[2].split(") AS v(reporter_found, reporter_standard)")[0]
statute = dict(re.findall(r"\('((?:[^']|'')*)', '(Stat\. [^']*)'\)", block))
statute = {k.replace("''", "'"): v for k, v in statute.items()}

rows = []
source = ""
with open(CANDIDATES, encoding="utf-8") as fh:
    for line in fh:
        line = line.rstrip("\n")
        if line.startswith("# source:"):
            source = line[len("# source:"):].strip()
        if not line.startswith("#TOJUNK\t"):
            continue
        parts = line.split("\t")
        if not parts[1].isdigit():
            continue  # the section's header line
        _, count, found, corrected, example = parts
        rows.append({
            "count": int(count),
            "found": ast.literal_eval(found),
            "corrected": ast.literal_eval(corrected),
            "example": ast.literal_eval(example),
        })

rows.sort(key=lambda r: (-r["count"], r["found"]))
seen = set()
rows = [r for r in rows if not (r["found"] in seen or seen.add(r["found"]))]
to_statute = [r for r in rows if r["corrected"] in statute]
to_junk = [r for r in rows if r["corrected"] not in statute]


def q(s):
    return s.replace("'", "''")


def clean(s):
    return s.replace("\n", " ").replace("\t", " ").strip()


sql = []
w = sql.append
w("-- migrate:up")
w("SET ROLE = law_admin;")
w("")
w("-- Seed legalhist.whitelist with the OCR-corrupted spellings whose every")
w("-- reading is a spelling the whitelist already rejects as junk (\"Cye.\" for")
w("-- \"Cyc.\", \"Dee.\" for \"Dec.\", \"Wol.\" for \"Vol.\", a lone \"ii\" for \"n\"),")
w("-- so that they stop counting as skipped_not_whitelisted, which reads as")
w("-- work still to do, and are recorded as the noise they are (issue #247,")
w("-- part of #165).")
w("--")
w("-- Candidates are the TOJUNK section of %s," % CANDIDATES)
w("-- compiled by TestOCRVariantWhitelistSuggestions from")
w("-- %s." % (source or "moml_citations.citations_unlinked"))
w("-- Readings are the same as for the reporter proposals: a letter-flanked")
w("-- digit stripped or read as its look-alike letter, or one substitution")
w("-- from the OCR confusion table in go/citations/ocrvariant.go. A spelling")
w("-- is here only when no reading reaches a reporter, so there is nothing to")
w("-- weigh it against; it inherits the whitelist's verdict on what it was")
w("-- read as. Built by scripts/build-whitelist-junk-migration.py.")
w("--")
w("-- %d spellings, %s rows. %d of them (%s rows) read as a regnal-year" % (
    len(rows), format(sum(r["count"] for r in rows), ","), len(to_statute),
    format(sum(r["count"] for r in to_statute), ",")))
w("-- statute spelling that 20260905120000_seed-statute-reporters.sql reroutes")
w("-- from junk to a statute row, and are seeded to that row instead, so the")
w("-- two migrations agree; the other %d (%s rows) are seeded as junk." % (
    len(to_junk), format(sum(r["count"] for r in to_junk), ",")))
w("-- Counts are rows in moml_citations.citations_unlinked.")
w("")
w("INSERT INTO legalhist.whitelist (reporter_found, reporter_standard, junk) VALUES")
all_rows = [(r, statute[r["corrected"]]) for r in to_statute] + [(r, None) for r in to_junk]
for i, (r, std) in enumerate(all_rows):
    end = "," if i < len(all_rows) - 1 else ""
    if std:
        w("    ('%s', '%s', false)%s  -- %d, read as %s, e.g. %s" % (
            q(r["found"]), q(std), end, r["count"], clean(r["corrected"]), clean(r["example"])))
    else:
        w("    ('%s', NULL, true)%s  -- %d, read as %s, e.g. %s" % (
            q(r["found"]), end, r["count"], clean(r["corrected"]), clean(r["example"])))
w("ON CONFLICT (reporter_found) DO NOTHING;")
w("")
w("-- migrate:down")
w("SET ROLE = law_admin;")
w("")
w("-- Remove exactly the rows seeded above, and only while they still carry")
w("-- the classification this migration gave them.")
w("DELETE FROM legalhist.whitelist")
w(" WHERE (junk OR reporter_standard LIKE 'Stat. %')")
w("   AND reporter_found IN (")
for i, (r, _) in enumerate(all_rows):
    end = "," if i < len(all_rows) - 1 else ""
    w("    '%s'%s" % (q(r["found"]), end))
w(");")
w("")

with open(OUT, "w", encoding="utf-8") as fh:
    fh.write("\n".join(sql))

print("TOJUNK spellings:   %d (%d rows)" % (len(rows), sum(r["count"] for r in rows)))
print("  to statute rows:  %d (%d rows)" % (len(to_statute), sum(r["count"] for r in to_statute)))
print("  to junk:          %d (%d rows)" % (len(to_junk), sum(r["count"] for r in to_junk)))
