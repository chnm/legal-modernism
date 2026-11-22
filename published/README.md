# published/

The project's published work, kept here for reference: what we have already said in print, and the data and code that stand behind it. Nothing in this directory is built or read by the programs in the repository. The files are as published and should not be edited.

Each publication has a directory.

- `spine/`: Kellen Funk and Lincoln A. Mullen, "The Spine of American Law: Digital Text Analysis and U.S. Legal Practice," *American Historical Review* 123, no. 1 (February 2018): 132–164, <https://doi.org/10.1093/ahr/123.1.132>.
  - `zay132.pdf` is the article.
  - `compendium_zay132/` is its research compendium, as the journal published it; its `README.txt` describes each file. It holds the corpus of codes of civil procedure, whole and split into sections; the tables of section matches and the clusters of sections; the two R packages written for the analysis; and `field-code-analysis.zip`, a copy of the analysis repository (<https://github.com/lmullen/civil-procedure-codes>).
  - `scripts/field-code-network` derives the data of chambers' Field Code network (`/viz/field-code`) from two files inside those zips: `best_matches.csv` in `best-section-matches.csv.zip`, and `article/figure-05-handedited-2.svg` in `field-code-analysis.zip`.

The files are large (about 225 MB in all, the largest 95 MB), and GitHub refuses a file over 100 MB, so a bigger one needs another home.
