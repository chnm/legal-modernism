# scripts/

One-off R, Python, and SQL scripts for data manipulation and import, and
`pipeline.sh`, which is not one-off and is documented here.

## pipeline.sh: the citation rebuild in one command

`pipeline.sh` rebuilds all citation data from a workstation with ssh access to
hopper, the HPC cluster (issue #321). Before it existed, a rebuild was five
hand-offs across two machines: sync the programs, log in to hopper, submit the
detector, notice when it finished, submit the linker, notice when it finished,
then run the local database steps. The script does all of it, waits where
waiting is needed, and fails loudly.

```
caffeinate -i ./scripts/pipeline.sh
```

Run it from the repository root. It takes about four hours, most of it the
detector. `caffeinate -i` keeps a laptop from sleeping; the script survives a
sleep, but the local steps after a job wait for the laptop to wake.

### What it does

| Phase | Runs | Where | Takes |
|---|---|---|---|
| `preflight` | Checks tools, database, migrations, ssh, hopper's environment, and the queue; asks once before the truncates | local and hopper | seconds |
| `sync` | `make sync-hopper`: build the linux binaries, rsync them and `slurm/` to hopper | local | a minute |
| `truncate-citations` | `TRUNCATE moml_citations.citations_unlinked CASCADE`, which also empties `citation_links` | local psql | seconds |
| `detect` | `sbatch` `cite-detector-moml`, wait, fetch its log | hopper | about 3h36m |
| `link` | `sbatch` `cite-linker`, wait, fetch its log | hopper | about 10 minutes |
| `stubs` | `make db-stubs`: rebuild `legalhist.stub_cases` from the linker's misses | local psql | a few minutes |
| `truncate-links` | `TRUNCATE moml_citations.citation_links` | local psql | seconds |
| `relink` | `sbatch` `cite-linker` again, so citations to reporters no source covers link to the stubs | hopper | about 10 minutes |
| `maintenance` | `make db-maintenance`: vacuum the churned tables, refresh every materialized view | local psql | minutes |

The database steps are the existing Makefile targets, run locally against
`LAW_DBSTR`. The Slurm jobs are the existing scripts in `slurm/`, unchanged.
The order and the reasons for it are in the "Pipeline run order" section of
`CLAUDE.md`.

### Prerequisites

On the workstation:

- `git`, `make`, `go`, `rsync`, `ssh`, `psql`, and `dbmate` on the `PATH`.
  The preflight names any that are missing.
- `LAW_DBSTR` set to the write-access connection string.
- An ssh config entry named `hopper` that authenticates without a prompt (a
  key in an agent). The script runs ssh with `BatchMode=yes`, so a password
  prompt is a failure, not a pause. `make sync-hopper` uses the same alias.
- No pending migrations: `make db-up`, then `make db-schema`, and commit
  `db/schema.sql`. The preflight refuses to start otherwise.

On hopper:

- `LAW_DBSTR` exported from the login shell's rc file (`~/.bash_profile` or
  what it sources). Nothing in the slurm scripts sets it: `sbatch` carries the
  submitting shell's environment into the job, and the script submits through
  a login shell so that this works. The preflight checks it.
- `sbatch`, `squeue`, and `sacct` on the login shell's `PATH`.
- `/scratch/$USER/logs`, where the slurm scripts write their output. The
  preflight creates it.

### Options

```
--from PHASE    resume at PHASE; earlier phases are skipped (preflight always runs)
--job ID        with --from detect, link, or relink: attach to Slurm job ID
                instead of submitting a new one
--skip-sync     do not run make sync-hopper
--dry-run       print every local and remote command; execute nothing, write nothing
--yes, -y       skip the confirmation that guards the TRUNCATE statements
--poll SECONDS  how often to poll squeue while a job runs (default 60)
-h, --help      show the usage
```

`STUB_THRESHOLD` in the environment passes through to `make db-stubs`
(default 5).

### Recipes

The full rebuild, after a change to the detector or its inputs:

```
caffeinate -i ./scripts/pipeline.sh
```

Relink everything after a whitelist or linker change, without re-detecting:

```
./scripts/pipeline.sh --from truncate-links
```

A routine incremental link of new citations, then maintenance:

```
./scripts/pipeline.sh --from link
```

See what a run would do without doing any of it:

```
./scripts/pipeline.sh --dry-run
```

Reattach to a detector job that is still running after the script was
interrupted, and carry on from there:

```
./scripts/pipeline.sh --from detect --job 12345
```

### The confirmation

A run that includes a truncate prints the statements it will execute and asks
once, at the end of preflight, before anything is changed. `--yes` answers for
you, for a run started from a script or left unattended from the start. The
question comes at the start rather than at each truncate because
`truncate-links` fires about four hours in, when nobody is at the terminal.
There is no terminal to ask on under `nohup` or cron, so those runs need
`--yes`.

### How it waits on hopper

After `sbatch --parsable` returns the job id, the script polls
`squeue -h -j ID -o %T` once a minute over a fresh ssh connection each time,
logging every change of state and a heartbeat every ten minutes. It does not
hold one ssh session open with `sbatch --wait`: a held session dies when the
laptop sleeps or the network blips, and the run with it. A transient ssh
failure backs off and retries; ten failures in a row give up, with the job
still running and the command to reattach printed.

When the job leaves the queue, the script reads its final state from
`sacct -j ID -X -n -P -o State,ExitCode`, retrying for a minute while
accounting catches up, and falling back to `scontrol show job` if accounting
has nothing. Then it copies the job's stderr, the JSON log, and stdout from
`/scratch/$USER/logs/` on hopper into the run directory and checks that the
log ends with the program's "done" line, reporting the counts it carries
(`pages_processed` and `citations_saved` for the detector, `processed` for
the linker).

Pass or fail is the Slurm job state, not the program's exit code. The detector
exits 1 both on a real failure and on the SIGTERM that Slurm sends at the wall
time, so the exit code cannot tell the two apart; the state can (`FAILED`
against `TIMEOUT`). A job that ends `TIMEOUT` is resubmitted once, because
both programs commit as they go: the linker picks up where it stopped, and
the detector rescans every page but inserts nothing twice. A second timeout,
or any other state, fails the run.

### When it fails

The script exits non-zero and prints a banner naming the phase that failed,
the run directory, and the exact command that resumes the run, along with the
last twenty lines of the job log when a job failed. It rings the terminal
bell and, on macOS, posts a notification. The Slurm jobs also send their
usual email on start, end, and failure.

Resume with the printed `--from PHASE` command. Preflight runs again, the
skipped phases are listed, and the run picks up at the named phase. A phase
that failed partway is safe to repeat: the truncates are idempotent, the
linker skips citations already linked, the detector rescans every page but
inserts nothing twice (so a repeated `detect` still costs the full 3.5 hours),
`make db-stubs` upserts, and maintenance is maintenance.

Ctrl-C never cancels a Slurm job. The script prints the job id, how to watch
or cancel it by hand, and the `--from PHASE --job ID` command that reattaches
to it. The same command works when the laptop was shut down mid-job and the
job has since finished: the script finds no queue entry, reads the final
state from accounting, fetches the log, and carries on.

Refusals in preflight, and what to do:

| Message | Fix |
|---|---|
| missing local tools | Install them; `dbmate` and `psql` come from Homebrew. |
| cannot connect to the database | Check `LAW_DBSTR`, the VPN, and the database host. |
| migrations are pending | `make db-up`, `make db-schema`, commit `db/schema.sql`. |
| cannot ssh to hopper | Check the `hopper` ssh alias and that a key is loaded. |
| `LAW_DBSTR` is not exported in the login shell on hopper | Export it from `~/.bash_profile` on hopper. |
| a pipeline job is already queued or running | Wait for it, cancel it, or attach to it with `--from PHASE --job ID`. |

A truncate that waits more than sixty seconds for its lock fails rather than
hanging, with PostgreSQL's lock timeout error. Something else holds a lock on
the table, usually a psql or GUI session left inside a transaction; end it and
resume.

### Logs

Each run gets a directory `logs/pipeline/<timestamp>/`, ignored by git:

- `pipeline.log`, everything the run printed, with the commit and the number
  of uncommitted changes recorded at the top.
- `<jobid>-cite-detector-moml.log` and `<jobid>-cite-linker.log`, the JSON
  logs of the jobs, copied from hopper, one per job including resubmissions.
- The matching `.out` files, usually empty because the programs log to stderr.

Commands are logged as they run, prefixed `+`, with the connection string
shown as `$LAW_DBSTR` rather than its value.

### Why a shell script

It is glue around `make`, `psql`, `ssh`, and `sbatch`, with nothing to test in
Go; the maintainer asked for shell. It targets the bash 3.2 that ships with
macOS, like `db/maintenance.sh`: indexed arrays only, no `mapfile`, no
associative arrays. `shellcheck` passes, with the intentional findings
about strings meant for hopper's shell marked as such.
