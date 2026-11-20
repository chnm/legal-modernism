library(tidyverse)

raw <- read_csv("tmp/whitelist.csv", na = "NULL")

raw |>
  mutate(
    reporter_found = str_trim(reporter_found),
    statute = if_else(is.na(statute), FALSE, statute),
    uk = if_else(is.na(uk), FALSE, uk),
    junk = if_else(is.na(junk), FALSE, junk)
  ) |>
  distinct(reporter_found, .keep_all = TRUE) |>
  write_csv("tmp/reporter-whitelist.csv", na = "")
