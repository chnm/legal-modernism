# CAP has multiple parallel cites in a single field. E.g.,
#   3 Sumn. 189; 3 Law Rep. 374
# Those need to be split apart.

library(tidyverse)
library(DBI)

db <- dbConnect(odbc::odbc(), "Research", timeout = 10)

raw_cites <- tbl(db, Id(schema = "cap", table = "citations")) |>
  filter(type == "parallel") |>
  filter(str_detect(cite, ";")) |>
  collect()

split_cites <- raw_cites |>
  mutate(split = str_split(cite, "[;|:]")) |>
  unnest(split) |>
  mutate(split = str_trim(split)) |>
  mutate(type = "split") |>
  select(-cite) |>
  select(cite = split, type, case) |>
  filter(cite != "")


write_csv(split_cites, "tmp/split-parallel-cites.csv")


dbDisconnect(db)
