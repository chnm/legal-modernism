SELECT cites.n, cites.volume, cites.reporter_standard, cites.page, cites.cleaner_cite,
vols.cap_vol || ' ' || abbr.cap_abbr || ' ' || cites.page AS altvol_cite
FROM linking.moml_01_clean_cites_agg AS cites
LEFT JOIN legalhist.reporters_alt_diffvols_abbreviations abbr ON cites.reporter_standard = abbr.alt_abbr
LEFT JOIN legalhist.reporters_diffvols vols ON abbr.reporter_title = vols.reporter_title AND cites.volume = vols.vol;