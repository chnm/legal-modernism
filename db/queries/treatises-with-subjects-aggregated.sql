SELECT v.psmid, s.subject, v.year, v.display_title, v.product_link FROM
(SELECT bibliographicid, array_agg(subject ORDER BY position) AS subject
FROM moml.edition_subjects
GROUP BY bibliographicid
HAVING 'UK' != ALL(array_agg(subject))) s
JOIN moml.volumes v ON s.bibliographicid = v.bibliographicid
ORDER BY v.year;
