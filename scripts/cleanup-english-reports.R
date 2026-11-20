library(tidyverse)
library(lubridate)

raw <- read_csv(
  "tmp/english_reports_cites.csv",
  na = "NULL",
  col_types = cols(
    er_filename = col_character(),
    er_id = col_character(),
    er_name = col_character(),
    er_url = col_character(),
    er_cite = col_character(),
    er_cite_disambiguated = col_character(),
    er_parallel_cite = col_character(),
    murrell_uid = col_character(),
    er_date = col_character(),
    murrell_year = col_character(),
    er_year = col_double(),
    murrell_title = col_character(),
    er_title = col_character(),
    court = col_character(),
    word_count = col_character()
  )
)

raw |>
  rename(er_date_raw = er_date) |>
  mutate(er_date = dmy(er_date_raw)) |>
  mutate(er_name = if_else(er_name == "", NA_character_, er_name)) |>
  select(-er_date_raw) |>
  select(
    id = er_id,
    er_name,
    er_year,
    er_date,
    er_cite,
    er_cite_disambiguated,
    er_parallel_cite,
    murrell_uid,
    murrell_year,
    murrell_title,
    everything()
  ) |>
  select(-er_title) |>
  filter(!is.na(er_date)) |>
  write_csv("tmp/english-reports-cleaned.csv", na = "")
