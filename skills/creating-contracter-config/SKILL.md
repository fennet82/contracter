---
name: creating-contracter-config
description: Use when a repository needs a contracter.yaml, when asked to set up or adopt contracter, to put a repo's config files under a contract, or to bring an existing contracter.yaml up to date with config files that were added since.
---

# Creating a contracter config

## Overview

`contracter.yaml` lists a repo's config files. `contracter snapshot` turns it into `config.contract.json`: which files exist, their key paths and value types. Another copy of those files (another environment) is later checked against it with `contracter check`.

**Core principle:** listing a file costs nothing, because shape checks hold in every environment. Every mark on a key (`optional`, `env-specific`, `locked`, `rules`) is a claim about environments you cannot see. Apply the marks the repo proves, and propose the rest.

Needs the `contracter` binary. If it is not on PATH and you cannot build it (`go build -o contracter ./cmd/contracter` in the contracter repo), stop and say so.

## Steps

1. **Detect.** Run `contracter init`. It writes a starter `contracter.yaml` listing every file in a config format; keep its header comment, which documents the syntax. If a `contracter.yaml` already exists, run `contracter init -config contracter.detected.yaml`, merge what is new, and delete that file.
2. **Prune.** Keep files that configure how the software runs or is deployed, and files whose structure must stay the same in every copy (locales, templates such as `.env.example`). Remove test fixtures, API specs and docs, CI workflow definitions, and package or chart metadata. When unsure, keep the file and say so in the report. Removals that would come back on the next `init` (a fixtures directory, generated files) also go in `.contracterignore`: one name or root-relative path per line, `name/` for directories, `!name` to bring back something left out by default.
3. **Group.** `init` already writes directories that agree with each other as one `{name}` entry. Add by hand what it cannot see:
   - sibling files: `deploy/values-{env}.yaml`
   - directories that *almost* agree: group them anyway; the difference is a finding (step 5)
4. **Mark keys**, using the table below. Run `contracter snapshot` first and read key paths from `config.contract.json`; do not guess them.
5. **Snapshot.** Run `contracter snapshot`. It writes the contract even when it exits 1.
   - A `matches no key` warning means a pattern is wrong. Fix it.
   - Exit 1 with deviations means the instances of a `{name}` entry disagree. That is a real gap in the repo. Report it. Do not edit the config files, mark the key `optional`, or ungroup the entry to make it pass.
   - Each file's `env_specific` and `locked` sections in the contract list the keys your marks actually bound to.
6. **Verify** with `contracter snapshot -check`. It must not print `contract is stale`; the only deviations allowed are the gaps from step 5.
7. **Report** (see below). Leave committing to the user.

Do not judge the result with `contracter check` in the repo you snapshotted. It compares the repo with itself, so every `env-specific` key reports `value unchanged`. That is the mark working.

## Marking keys

An unmarked key must exist with the same type; its value is free.

```yaml
files:
  - deploy/values-{env}.yaml          # a bare path: shape only
  - path: config/app.yaml
    optional: [debug]                 # the section and everything under it
    env-specific: [db.host, "*.password"]
    locked: [schema_version]
    rules:
      - {key: "*.url", match: '^https://'}
      - {key: image, not: 'docker\.io'}
```

| Mark | A check fails when | Apply when the repo shows | Otherwise |
|---|---|---|---|
| `optional` | never, for those keys | the key is present in some sibling files and absent in others on purpose | propose; a section merely named `debug` is not proof |
| `env-specific` | the value is still the snapshot's | the value names *this* environment: its hostname, its URL, its bucket, its credentials | propose |
| `locked` | the value is not the snapshot's | nothing; only the user can say a value must be identical everywhere | propose, and never for what moves per release or environment (image tags, replica counts, limits) |
| `rules` | a value breaks the regexp | nothing; rules encode policy the user stated | propose; the starter file's rules are examples |

Three cases need care:

- **Shared services.** Registries, module sources and public endpoints do not name this environment. Whether the other environment mirrors them decides `env-specific` versus `locked`, and the repo cannot tell you. Ask. If nobody can answer, leave them unmarked and propose.
- **`{name}` entries.** The values of all instances are pooled. `env-specific` then fails the repo's own files, so do not use it there; propose instead. `locked` only means "one of the instances' values".
- **Secrets.** Hashes of marked values go into the contract unsalted. Mark a secret only if its plaintext is already tracked by git in this repo. Do not mark keys in an untracked file such as a gitignored `.env`.

An empty value marked `env-specific` means "must be filled in elsewhere".

## Report

- Entries kept, entries removed, and why for each removal
- Marks applied, each with the evidence in the repo
- Proposed marks that need a human decision
- Gaps `snapshot` found
- Files changed (only `contracter.yaml` and `config.contract.json`)

## Quick reference

| Need | How |
|---|---|
| Key paths | `db.host`, `servers[].name`; Terraform `module.vpc.source`; Kubernetes `Deployment/web.spec.replicas` |
| Key patterns | `*` matches any run of characters; a pattern also covers everything under it |
| Path patterns | `*`, `?`, `**`; `{name}` means every match must have the same files and keys |
| Config not stored as a repo file | keep its entry; `contracter snapshot -source secrets/app.env=json:<(command)` |
| Exit status | 0 ok, 1 deviations, 2 error |
| Flags | `contracter -h` |
