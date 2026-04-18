#!/usr/bin/env Rscript

# Extracts one row per entry from the downloaded Cardiff Index to Legal
# Abbreviations search result pages in out/legal-abbreviations/ and writes
# out/legal-abbreviations/legal-abbreviations.csv with columns
# title, jurisdiction, abbreviations, url.
#
# Jurisdiction is a comma-separated list in the source (e.g. "United States,
# Kentucky" or "England & Wales"). It collapses to one of three codes:
#   us = any "United States" token present and no UK token
#   uk = any "United Kingdom" / "England & Wales" / "England & Ireland"
#        token present and no US token
#   mc = both US and UK tokens present (multi-country)
#
# XPath is used instead of CSS selectors because rvest's CSS-to-XPath
# translator (via selectr) fails to match `div.teaser` on these pages.

suppressPackageStartupMessages({
  library(rvest)
  library(xml2)
  library(dplyr)
  library(purrr)
  library(stringr)
  library(readr)
  library(tibble)
})

in_dir  <- "out/legal-abbreviations"
out_csv <- file.path(in_dir, "legal-abbreviations.csv")

UK_TOKENS <- c("United Kingdom", "England & Wales", "England & Ireland")

juris_code <- function(x) {
  tokens <- str_trim(str_split(x, ",", simplify = FALSE)[[1]])
  has_us <- any(tokens == "United States" |
                  str_starts(tokens, "United States "))
  # State-only tokens (e.g. "Kentucky", "Louisiana") always appear alongside a
  # "United States" token in this dataset, so we don't need to enumerate them.
  has_uk <- any(tokens %in% UK_TOKENS)
  if (has_us && has_uk) "mc"
  else if (has_us)      "us"
  else if (has_uk)      "uk"
  else                  NA_character_
}

field_after <- function(teaser, label) {
  ps <- xml_find_all(teaser, ".//p")
  txt <- xml_text(ps)
  hit <- txt[str_detect(txt, fixed(paste0(label, ":")))]
  if (length(hit) == 0) return(NA_character_)
  str_trim(str_remove(hit[[1]], fixed(paste0(label, ":"))))
}

parse_page <- function(path) {
  doc <- read_html(path)
  teasers <- xml_find_all(doc, "//div[@class='teaser']")
  title_a <- map(teasers, ~ xml_find_first(.x, ".//h2[@class='teaser-entry-title']/a"))
  tibble(
    title         = map_chr(title_a, ~ xml_text(.x, trim = TRUE)),
    url           = map_chr(title_a, ~ xml_attr(.x, "href")),
    abbreviations = map_chr(teasers, field_after, label = "Abbreviations"),
    jurisdiction  = map_chr(teasers, ~ juris_code(field_after(.x, "Jurisdiction")))
  )
}

files <- sort(list.files(in_dir, pattern = "^page-\\d{3}\\.html$", full.names = TRUE))
stopifnot(length(files) == 172)

rows <- map_dfr(files, parse_page) |>
  select(title, jurisdiction, abbreviations, url)

stopifnot(nrow(rows) == 4284L)
stopifnot(!any(is.na(rows$jurisdiction)))
stopifnot(!any(is.na(rows$title)))
stopifnot(!any(is.na(rows$url)))
stopifnot(all(startsWith(rows$url, "https://legalabbrevs.cardiff.ac.uk/record/")))

write_csv(rows, out_csv)
message(sprintf("wrote %d rows to %s", nrow(rows), out_csv))
