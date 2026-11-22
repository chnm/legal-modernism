# llm-sampler

Claude reads random body pages of the MOML treatises and records the cases
they cite (issue #474). It follows the page-by-page reading of Stephen on
Pleading (`llm-reader`, issue #439), with two differences:

- **It is designed for the Claude API.** The Stephen readers queried the
  database freely: other editions, name searches, whatever they thought of. A
  page here is two stateless requests with everything the model may use
  inside them, and ordinary code in between.
- **It uses the pipeline.** The detector's citations and the linker's results
  for the page go into the first request, and the pipeline's link is one of
  the candidates in the second. The aim is the best reading, not an
  independent measure of the pipeline.

For now the requests are answered by a Claude Code subagent standing in for
the API (`READER.md`), one batch of ten pages at a time. Nothing is written
to the database: the records are files under the data directory.

## The sample

Pages are drawn uniformly over editions: a random official copy in
`moml.treatises`, then a random `bodyPage` among its volumes. A long treatise
and a short one are equally likely, so the sample shows more books than a
draw over pages would. Draws are with replacement over editions (a page
already drawn is skipped), and follow from the seed alone: draw *n* hashes
`seed|n|edition` and `seed|n|page`, so the sample can be continued or rebuilt
in order. Editions whose official copy is a HathiTrust scan have no page
types and are passed over.

## The two requests

Both go to `claude-opus-5-5` with `output_config.format` set to a JSON
schema, so the response is the JSON object and nothing else. The system
prompt is the same for every page and carries `cache_control`, so that after
the first request it is read from the cache. Since nothing is waiting on the
answer, the requests suit the Message Batches API: all the first requests as
one batch, then the candidates step, then all the second requests as another.

| | First request: detect | Second request: link |
|---|---|---|
| System prompt (cached) | `system-detect.md`: what counts as a citation of a case, how to read the OCR, and the table of every reporter in `legalhist.reporters` (about 20,000 tokens) | `system-link.md`: how to choose among candidates |
| User message | `detect/<id>.request.json`: the edition, the page's OCR text, the end of the page before and the start of the page after, and the pipeline's detections with their links | `link/<id>.request.json`: the edition, the page's text, and each citation with its candidates |
| Response | `detect/<id>.response.json`: each citation as read (name, volume, reporter, the reporter's standard abbreviation, page, alternate readings of doubtful digits, whether the pipeline had it), and the pipeline detections rejected | `link/<id>.response.json`: for each citation, the candidate chosen or none, whether it is a pin cite, and a confidence |

Between them, `candidates` looks each citation up, with nothing but exact
matches on indexed columns and one name search:

- **By cite.** The reporter is the one the first request chose, and whatever
  `legalhist.whitelist` makes of the printed spelling, fallback included.
  Cite strings are built as the linker builds them (standard spelling, CAP's
  spelling, CAP's renumbered volume, the volume added or dropped for a
  single-volume reporter, the year for a reporter cited by year) for every
  page from 40 before the cited page to 3 after, and probed in
  `cap.citations`, `english_reports.cases`, `legalhist.code_reporter` and
  `legalhist.stub_cases`. A case is offered if it begins at the page (`exact
  cite`), within three pages of it, or, in CAP, runs over it (`page inside
  case`). The English Reports give no last page, so the two nearest starts
  before the page are offered with the length of each report.
- **By alternate reading.** The exact cite of each alternate the first
  request gave.
- **By name.** The most distinctive word of each party, matched against CAP
  and English Reports names, for cases decided no later than the book;
  cases in the cited volume come first, then the cited reporter.
- **The pipeline's link**, marked as such.

`finish` checks that every choice is one of the candidates offered and writes
the final record. A link can therefore only be to a case code found; a
citation none of whose candidates is right is `no_match`.

## Running it

```sh
go build -o bin/llm-sampler ./llm-sampler/
LAW_DBSTR="$LAW_CLAUDE" bin/llm-sampler sample                # draw the next ten pages, write the detect requests
LAW_DBSTR="$LAW_CLAUDE" bin/llm-sampler candidates --batch N  # after the detect responses: write the link requests
bin/llm-sampler finish --batch N                              # after the link responses: write the records
bin/llm-sampler serve --port 4313                             # the dashboard
```

The data directory (`--dir`, default `tmp/llm-sampler`, ignored by Git) holds:

- `system-detect.md`, `system-link.md`: the system prompts as sent.
- `batches/<NNNN>/batch.json`: the pages drawn, and when the batch started
  and finished.
- `batches/<NNNN>/detect/` and `link/`: each page's requests and responses.
- `results/<psmid>/<pageid>.json`: the final record of a page, in the shape
  of `llm-reader`'s records with more fields: how the citation stands to the
  pipeline's (`same`, `corrected`, `new`), the confidence of the reading and
  of the link, the linked case, how its candidate was found, whether it is
  the pipeline's link, and the pipeline detections rejected.

The coordinating session runs `sample`, gives a reader subagent `READER.md`
with the batch number, and when the reader reports back draws the next batch.
The dashboard's active time is the time batches were open (from `sample` to
`finish`); its wall clock runs from the first batch's start to the last
batch's finish, or to now while one is open.
