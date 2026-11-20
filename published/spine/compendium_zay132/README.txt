# Research compendium for "The Spine of American Law"

This research compendium contains supplementary materials for Kellen Funk and 
Lincoln A. Mullen, "The Spine of American Law: Digital Text Analysis and U.S. 
Legal Practice," _American Historical Review_ 123, no. 1 (February 2018). This 
compendium is intended to substantiate our findings by allowing researchers to 
inspect our code and reproduce our computational results, while also allowing 
researchers to take our investigations further by using our sources, code, or 
results for themselves.

Here are descriptions of the pieces of this compendium:

- `field-code-analysis.zip` contains the code to create the analysis and 
  visualizations for this article. See the `README` in that file for 
  instructions on how to reproduce the research. The analysis repository is 
  available at <https://github.com/lmullen/civil-procedure-codes/>.

- `textreuse_0.1.4.tar.gz` and `tokenizers_0.1.4.tar.gz` are two R packages 
  written to generalize our analysis. These packages have been separately 
  peer-reviewed by rOpenSci. The repositories for these packages are available 
  at <https://github.com/ropensci/textreuse> and 
  <https://github.com/ropensci/tokenizers>.

- `procedure-codes.zip` and `procedure-code-sections.zip` contain the corpus of 
  sources that we used in our research. The first ZIP file contains one OCR 
  text file for each code or statute in our corpus. The citations and sources 
  for these files can be found here: 
  <http://kellenfunk.org/civil-procedure/procedure-law/>. The second ZIP 
  contains those procedure codes programmatically split into sections, one text 
  file per section.

- `all-section-matches.csv.zip` and `best-section-matches.csv.zip` contain 
  tables showing the matches and similarity scores between sections of the 
  procedure codes. The file containing the best matches has been filtered as 
  explained in the article and documented in the analysis repository to limit 
  matches to at most one for each section in a borrowed code.

- `clusters-of-sections.zip` contains the results of clustering sections of 
  procedure codes. These clusters document changes in sections of procedure 
  codes.

These files are up to date as of the date of publication in February 2018.

