-- Editions left out of moml.treatises, and why (issue #142).
--
-- Each MOML edition that has a jurisdiction but is not in moml.treatises,
-- with the rule that removed it, its length, and the citations detected in
-- it, so the rules can be reviewed. The rules are stated in
-- db/migrations/20260925160000_treatises-exclude-pamphlets-and-documents.sql;
-- the patterns here must be kept in step with the view. Where several rules
-- apply, the first listed wins. Read-only; run with LAW_CLAUDE.

WITH edition AS (
  SELECT v.bibliographicid, min(v.display_title) AS title, sum(v.total_pages) AS pages
  FROM moml.volumes v
  GROUP BY v.bibliographicid
),
cites AS (
  SELECT v.bibliographicid, count(*) AS n
  FROM moml_citations.citations_unlinked cu
  JOIN moml.volumes v ON v.psmid = cu.moml_treatise
  GROUP BY v.bibliographicid
),
excluded AS (
  SELECT e.*, j.subject AS jurisdiction,
    CASE
      WHEN EXISTS (SELECT 1 FROM moml.edition_subjects s
                   WHERE s.bibliographicid = e.bibliographicid
                     AND s.subject IN ('Biography', 'Collected Essays', 'Trials'))
        THEN 'Gale subject'
      WHEN e.title ~* '\Wremarks of\W'
        OR e.title ~* '\y(address|oration|eulogy|sermon|memorial|in memoriam|obituary)\y'
        THEN 'commemorative title'
      WHEN coalesce(e.pages, 50) < 50
        THEN 'under 50 pages'
      WHEN e.title ~* (
             '^(the )?((first|second|third|fourth|fifth|final|annual|special|preliminary|majority|minority|supplementary) )*reports? (of|from|to) (the )?(\w+ ){0,6}(committee|commission|commissioners|board|council|delegates|attorney|secretary|comptroller|superintendent)'
          || '|^(the )?(hearings?|message|debates?|journal of the|proceedings of the (senate|house|convention|legislature))\y'
          || '|^(the )?(argument|closing argument|opening argument|brief|reply brief)s? (of|for|on behalf of|in|by|against|submitted)\y'
          || '|^in the (supreme )?court\y'
          || '|(^|: )(the )?speech(es)? (of|delivered|in the|on)\y'
          || '|(^|: )(a |an |the )?(second |third |open |plain )?letters? (to|addressed to)\y')
        THEN 'document title'
      ELSE 'LoC biography or speeches'
    END AS reason
  FROM edition e
  JOIN moml.edition_subjects j
    ON j.bibliographicid = e.bibliographicid AND j.subject IN ('US', 'UK')
  WHERE NOT EXISTS (SELECT 1 FROM moml.treatises t WHERE t.bibliographicid = e.bibliographicid)
)
SELECT x.reason, x.jurisdiction, x.bibliographicid, x.pages,
       coalesce(c.n, 0) AS citations, x.title
FROM excluded x
LEFT JOIN cites c USING (bibliographicid)
ORDER BY x.reason, citations DESC, x.title;
