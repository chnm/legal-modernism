#!/usr/bin/env Rscript

# Extracts one row per entry from the downloaded Cardiff Index to Legal
# Abbreviations search result pages in out/legal-abbreviations/ and writes
# out/legal-abbreviations/legal-abbreviations.csv with columns
# title, jurisdiction, abbreviations, url.
#
# Jurisdiction codes:
#   us = United States
#   uk = United Kingdom
#   mc = multi-country (both United Kingdom and United States)

suppressPackageStartupMessages({
  library(rvest)
  library(dplyr)
  library(purrr)
  library(stringr)
  library(readr)
  library(tibble)
})

in_dir  <- "out/legal-abbreviations"
out_csv <- file.path(in_dir, "legal-abbreviations.csv")

juris_code <- function(x) {
  x <- str_trim(x)
  dplyr::case_when(
    x == "United States"                 ~ "us",
    x == "United Kingdom"                ~ "uk",
    x == "United Kingdom, United States" ~ "mc",
    TRUE                                 ~ NA_character_
  )
}

field_after <- function(teaser, label) {
  ps <- html_elements(teaser, "p")
  txt <- html_text(ps)
  hit <- txt[str_detect(txt, fixed(paste0(label, ":")))]
  if (length(hit) == 0) return(NA_character_)
  str_trim(str_remove(hit[[1]], fixed(paste0(label, ":"))))
}

parse_page <- function(path) {
  doc <- read_html(path)
  teasers <- html_elements(doc, "div.teaser")
  title_a <- html_element(teasers, "h2.teaser-entry-title a")
  tibble(
    title         = html_text(title_a, trim = TRUE),
    url           = html_attr(title_a, "href"),
    abbreviations = map_chr(teasers, field_after, label = "Abbreviations"),
    jurisdiction  = juris_code(map_chr(teasers, field_after, label = "Jurisdiction"))
  )
}

files <- sort(list.files(in_dir, pattern = "^page-\\d{3}\\.html$", full.names = TRUE))
stopifnot(length(files) == 76)

rows <- map_dfr(files, parse_page) |>
  select(title, jurisdiction, abbreviations, url)

stopifnot(nrow(rows) == 1891L)
stopifnot(!any(is.na(rows$jurisdiction)))
stopifnot(!any(is.na(rows$title)))
stopifnot(!any(is.na(rows$url)))
stopifnot(all(startsWith(rows$url, "https://legalabbrevs.cardiff.ac.uk/record/")))

write_csv(rows, out_csv)
message(sprintf("wrote %d rows to %s", nrow(rows), out_csv))
