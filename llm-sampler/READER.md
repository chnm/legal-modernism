# Reader brief

The coordinating session gives each reader subagent this brief with a batch
number. A reader stands in for the Claude API: it answers two requests for
each page of the batch, and nothing more.

---

You are standing in for the Claude API in a simulation (issue #442). A batch
is ten pages of nineteenth-century legal treatises. Each page gets two
requests, and you play the model answering each one: your only inputs are the
system prompt and the request, and your only output is the response file.

All paths are under `<DIR>` = `/Users/lmullen/github/chnm/legal-modernism/tmp/llm-sampler`.
Your batch is `<DIR>/batches/<NNNN>/`.

Rules of the simulation:

- Do not query the database, search the web, read the repository, or read
  other batches or `results/`. Everything you may use is in the system prompt
  and the request file. You may use what you know of legal history and
  citation practice, as the model would.
- Treat each request as independent: do not carry anything from one page's
  request into another's.
- Read every request file in full, with the Read tool. Write each response
  with the Write tool, as one valid JSON object and nothing else.
- Run only the two commands given below, exactly as written.

## Step 1: detect

Read `<DIR>/system-detect.md` once: it is the system prompt of the first
request, the same for every page. Then, for each
`detect/<id>.request.json`, write `detect/<id>.response.json` as the system
prompt directs.

When all ten are written, run:

```sh
LAW_DBSTR="$LAW_CLAUDE" /Users/lmullen/github/chnm/legal-modernism/.claude/worktrees/llm-random-pages-474/bin/llm-sampler candidates --batch <N>
```

This is the code between the two requests: it checks your responses and looks
up candidate cases for each citation. If it reports problems, fix those
response files and run it again.

## Step 2: link

Read `<DIR>/system-link.md` once: the system prompt of the second request.
Then, for each `link/<id>.request.json`, write `link/<id>.response.json` as it
directs. A page with no citations still gets a response, with `"links": []`.

When all ten are written, run:

```sh
/Users/lmullen/github/chnm/legal-modernism/.claude/worktrees/llm-random-pages-474/bin/llm-sampler finish --batch <N>
```

If it reports problems, fix those response files and run it again.

## Report

Reply in at most six lines: pages done, citations detected, how many linked,
and anything in either system prompt or in the requests that was unclear,
wrong, or missing something you needed (for example, a kind of citation the
rules do not cover, or candidates that should have been offered and were
not). Say nothing else.
