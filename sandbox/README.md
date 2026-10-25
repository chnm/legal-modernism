# Docker Sandbox kits

Kits for [Docker Sandboxes](https://docs.docker.com/ai/sandboxes/) (`sbx`),
written to the [Sandbox Kit specification v3](https://github.com/docker/sandbox-kit-spec/blob/main/docs/spec/SPEC-v3.md).
Each kit is a directory holding a `<name>.yaml` descriptor and, beside it, a
`<name>.dockerfile` recipe for the files it ships. A kit is applied to a sandbox
with `--kit`, from this checkout, from the repository over git, or from a
published image.

## claude-statusline

The repository's Claude Code status line (`.claude/statusline.sh`, which this
project's `.claude/settings.json` registers for sessions in this checkout),
packaged as a mixin for the built-in `claude` agent so that every sandbox gets
it, whatever project is checked out there:

```
🏖️ legal-modernism  ·  Opus 4.1  ·  legal-modernism/fix-linker  ·  session (resets 3h12m): 24% used / 36% over
```

- **sandbox** (yellow), from `SANDBOX_NAME`, which Docker Sandboxes export.
  Omitted when the variable is unset, so the script also reports correctly on a
  host installation of Claude Code.
- **model** (cyan).
- **project/branch** (magenta). The project is the name of git's main worktree,
  so a session that has entered a linked worktree still shows as
  `legal-modernism/<branch>` and not as the worktree folder. Long branch names
  are truncated. Outside a git repository the folder name is shown alone.
- **session**, coloured green, yellow or red by how much of the 5-hour quota is
  used, with the time until it resets and how far through the window it is.

The kit ships the script to `~/.claude/statusline.sh` and registers it under
`statusLine` in `~/.claude/settings.json`, merging into the file so the claude
base image's other settings are preserved. Only the `statusLine` key is written;
an existing one is replaced, which is the point of installing the kit.

### Quick start

From a checkout of this repository:

```console
sbx run claude --kit ./sandbox/claude-statusline .
```

From the repository over git, without a checkout:

```console
sbx run claude --kit "git+https://github.com/chnm/legal-modernism.git#dir=sandbox/claude-statusline" .
```

To publish it to Docker Hub as an OCI artifact and use it by reference, build the
descriptor with its BuildKit frontend (the `# syntax=docker/sandbox-kit:3` line
dispatches it) and push the result:

```console
docker buildx build -f sandbox/claude-statusline/claude-statusline.yaml \
  -t docker.io/<user>/sbx-kit-claude-statusline:1.0.0 --push sandbox/claude-statusline
sbx run claude --kit docker.io/<user>/sbx-kit-claude-statusline:1.0.0 .
```

### How it works

- `claude-statusline/files/home/.claude/statusline.sh` is the script. The
  recipe `claude-statusline.dockerfile` bakes it into the kit's overlay at
  `/home/agent/.claude/statusline.sh`, owned by the `agent` user (uid 1000) and
  executable, via a build stage that creates `/home/agent` with the right
  ownership rather than a bare `COPY`, whose root-owned parent directories would
  take `$HOME` away from the agent. The script receives the session JSON on
  stdin and prints the line.
- The `install` hook in `claude-statusline.yaml` (run as root, once, when the
  sandbox is created) merges the `statusLine` block into `~/.claude/settings.json`
  with `jq`, creating the file if missing and leaving every other key untouched,
  makes the script executable (without the bit Claude Code silently renders no
  status line), and `chown`s back to `agent` exactly the three paths it touched,
  not `~/.claude` recursively, since that directory holds runtime-managed content
  the kit does not own. Re-running it is idempotent.
- `requires: ["claude", "deb/jq"]`: the kit composes onto the `claude` agent,
  and both the hook and the script need `jq`, which the claude base image has.
  The script also uses `git`, `date`, `sed` and `basename`, all on the platform
  floor. The kit declares no network policy; it opens no socket.
- `claude-statusline-context.md` is a short note the sandbox adds to the
  agent's context so Claude knows what the status line shows.

### Keeping the two copies in step

`.claude/statusline.sh` (used by this project's own `.claude/settings.json`)
and `sandbox/claude-statusline/files/home/.claude/statusline.sh` (shipped by the
kit) are the same script, except that the kit's copy adds the sandbox-name
segment. When one changes, change the other, and bump `version:` in the
descriptor. Note that a project's `.claude/settings.json` overrides the user-level
`~/.claude/settings.json`, so in a sandbox on this repository the checkout's own
`statusLine` wins; the kit is for sandboxes on other projects.
