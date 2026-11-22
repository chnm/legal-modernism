# Cleaning up the textbook data so that it has the psmid and bibliographic ID
library(tidyverse)
library(DBI)
library(dbplyr)

db <- dbConnect(odbc::odbc(), "Research", timeout = 10)
book_info <- tbl(db, in_schema("moml", "book_info")) |> collect()
book_info <- book_info |>
  select(webid, psmid, bibliographicid)
raw_tb <- read_csv("tmp/procedure_textbooks.csv") |>
  select(-moml_psmid) |>
  rename(webid = moml_webid)

clean_tb <- raw_tb |>
  left_join(book_info, by = "webid") |>
  select(bibliographicid, psmid, webid, everything()) |>
  mutate(year_begin = as.integer(begin_year),
         year_end = as.integer(end_year)) |>
  select(-begin_year, -end_year)

clean_tb |>
  write_csv("tmp/procedure-textbooks-cleaned.csv")

dbWriteTable(db, Id(schema = "legalhist", table = "textbooks"), clean_tb)




