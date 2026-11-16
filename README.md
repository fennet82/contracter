# contracter

Contract testing for configuration files.

`contracter` records what a set of config files looks like (which files exist,
which keys they hold, what type each value is) as a **contract** committed
next to them. Any later copy of those files can then be checked against it:
another environment, another branch, a deployment directory, a rendered
secret. The check fails on every deviation, so config drift is caught in CI
instead of at runtime.

It compares *shape*, not values, so environments are free to hold different
values. Where values do matter you say so per key: this one must change, this
one must not, this one must look like that.

```
contracter snapshot      known-good config  ──▶  config.contract.json
contracter check         any copy of it     ──▶  list of deviations, exit 1
```

## Install

```sh
go build -o contracter ./cmd/contracter     # Go 1.25+
```

Dependencies are not committed. To build somewhere without network access,
run `go mod vendor` first and carry the `vendor/` directory along.

## Use

```sh
contracter init              # once: writes a starter contracter.yaml to prune
contracter snapshot          # writes config.contract.json; commit it
contracter check             # compares the files to the contract
```

```
$ contracter check
config/app.yaml                 db.host                value unchanged from snapshot (env-specific)
deploy/values.yaml              image.tag              value differs from snapshot (locked)
deploy/values.yaml              replicas               type changed: number -> string
deploy/values.yaml              resources.limits.cpu   missing key (string)
k8s/web.yaml                    Service/web.spec.port  missing key (number)
public/locales/he/billing.json                         file missing
6 deviation(s)
```

Exit status: `0` ok, `1` deviations, `2` error. Values are never printed.

## contracter.yaml

```yaml
files:
  - deploy/values.yaml                  # a bare path is enough

  - path: config/app.yaml
    optional: [debug.*]                 # may be absent or extra
    env-specific: [db.host, "*.password"]   # value must differ from the snapshot
    locked: [api.version, features]     # value must equal the snapshot
    rules:                              # constraints on values (regexps)
      - {key: "*.url", match: '^https://'}
      - {key: image, not: 'docker\.io'}

  - public/locales/{lng}/*.json         # layout, see below
  - deploy/values-{env}.yaml
```

| Field | Effect at `check` |
|---|---|
| `optional` | the key may be missing, and extra keys under it are accepted |
| `env-specific` | fails if the value is still the one seen at snapshot (a forgotten override) |
| `locked` | fails if the value is not the one seen at snapshot (unwanted drift) |
| `rules` | `match`: the value must match; `not`: it must not |
| `format` | overrides detection by file extension |

Key patterns address keys as `db.host` or `servers[].name`. `*` matches any
run of characters, and a pattern also covers everything nested under it.

`snapshot` warns when a pattern matches no key, since that is usually a typo
that would silently switch the check off.

### Layout

Paths may use `*`, `?` and `**`. A `{name}` placeholder means *everything
matching here must have the same files and the same keys*:

- `public/locales/{lng}/*.json`: every locale directory holds the same files
  with the same keys.
- `deploy/values-{env}.yaml`: every environment's values file has the same keys.

The contract remembers which instances existed (`en`, `he`). They must all
still exist, and a new one (`fr`) is accepted only if it conforms. `snapshot`
itself exits 1 when the instances disagree with each other; it still writes the
contract, which then holds the union of their keys.

`init` detects sibling directories that are already in parity and writes them
as one `{name}` entry.

## What init leaves out

`init` lists every file in a config format, except the ones that are not a
project's configuration: dependency and build directories (`node_modules`,
`vendor`, `dist`, …), tool state (`.git`, `.terraform`, …), and package
manifests and lock files. The full default list is
[`internal/discover/default.contracterignore`](internal/discover/default.contracterignore).

To change it for a repo, add a `.contracterignore` at the repo root. It is
read after the defaults, and the last line that matches wins:

```
# a directory with that name, at any depth
fixtures/
# a file name, at any depth
*.generated.json
# a path from the repo root
docs/*.yaml
# bring back something the defaults leave out
!vendor/
```

Helm `templates/` directories are always left out, since their files are not
YAML until rendered.

## What check reports

| Message | `kind` in JSON | Meaning |
|---|---|---|
| `missing key (type)` | `missing-key` | in the contract, not in the file |
| `unknown key (not in contract)` | `unknown-key` | in the file, not in the contract |
| `type changed: number -> string` | `type-changed` | the value's type differs |
| `value unchanged from snapshot` | `value-unchanged` | an `env-specific` key was never overridden |
| `value differs from snapshot` | `value-changed` | a `locked` key drifted |
| `value does not match …` / `matches forbidden …` | `rule` | a rule is violated |
| `file missing` / `unexpected file` | `file-missing` / `unexpected-file` | the layout changed |
| `cannot parse as …` | `unreadable` | the file is not valid in its format |
| `contract is stale` / `does not exist` | `stale-contract` | only from `snapshot -check`: the contract needs regenerating |

Things that are deliberately not deviations:

- List length. Only the shape of list elements is compared.
- Contents the contract never saw: an empty list or map in the contract accepts
  any contents, and a null accepts anything.
- Types, when either side is a string-only format (`.env`, INI, `.properties`, XML).

`-o json` prints `{"deviations": [{"file", "key", "kind", "message"}]}` on
stdout for other tools to consume.

## Configs that live somewhere else

A contract entry describes a config, not where it is stored. When a copy lives
outside the repo (a secret manager, a mounted directory, another checkout),
re-point it with `-source CONTRACT-PATH=[FORMAT:]ACTUAL-PATH` on `snapshot` or
`check`. The actual path may be a file, a directory or a pipe:

```sh
# the config is a file in one place and a secret-manager entry in another
contracter check -source secrets/app.env=json:<(vault kv get -format=json -field=data secret/app)

# a whole directory comes from a mount
contracter check -source public/locales=/mnt/locales
```

- contracter never runs commands or talks to a config service; pipe the data in.
- `FORMAT:` is needed when the data is not in the format the path implies.
- Values read through `-source` at snapshot time are not recorded, not even as
  hashes, so `env-specific` and `locked` have no effect for those files.
- The contract can only name paths inside the repo. Anything outside it has to
  be passed with `-source` by whoever runs the command.

## Keeping the contract up to date

```sh
contracter snapshot -check
```

Writes nothing and exits 1 if `config.contract.json` no longer matches the
config files or `contracter.yaml`, listing what moved. Run it in CI next to the
config it describes, so a config change cannot merge without its contract.

## Agent skill

`skills/creating-contracter-config/` is a skill that teaches a coding agent to
write `contracter.yaml` for a repo: detect the config files, drop what is not
config, group layouts, and mark only the values the repo gives evidence for.

Copy the folder to where your agent harness looks for skills, for example
`.claude/skills/` in the project (Claude Code) or `~/.agents/skills/`
(Codex, Copilot CLI, Gemini CLI), then ask the agent to set up contracter.

## Formats

| Format | Files | Notes |
|---|---|---|
| YAML | `.yaml`, `.yml` | multi-document files included; Kubernetes-style documents are keyed `Kind/name` so each manifest keeps its own shape |
| JSON | `.json` | |
| TOML | `.toml` | |
| HCL | `.tf`, `.tfvars`, `.hcl` | blocks become paths such as `variable.region.default`, `module.vpc.source`; non-literal values have type `expression`, compatible with anything |
| dotenv | `.env`, `.env.*`, `*.env` | strings only |
| INI | `.ini`, `.cfg` | strings only |
| Properties | `.properties` | strings only |
| XML | `.xml` | attributes are `@name`; strings only |

Not supported: config written as code (Pulumi, `*.config.js`), templates that
need rendering first (Helm `templates/`, Jinja), and free-form files without a
key/value grammar (nginx.conf, Dockerfile).

## What is in the contract

`config.contract.json` holds file paths, key paths, types, your rules, and
SHA-256 hashes of the values of `env-specific` and `locked` keys. It holds no
other values. The hashes are unsalted, but they are only ever taken from files
in the repo, so the contract reveals nothing the repo does not already hold.
Keep that in mind if you publish the contract without the config files.

## Code layout

```
cmd/contracter      entry point
internal/cli        arguments and flags, commands, output, exit status
internal/discover   finds a repo's config files for `init`; the default ignore list
internal/contract   contracter.yaml, the contract file, building one, checking against one
internal/files      path patterns, -source re-pointing, reading files from disk
internal/format     one file per config format
internal/shape      key paths, types and how two sets of keys are compared
skills/             the agent skill described above
```

Each Go package imports only the packages listed below it. The tests drive the
command line end to end (`go test ./...`).

## Limits

- A key that itself contains a dot is indistinguishable from nesting in a key path.
- A list mixing element types is recorded with the first concrete type seen.
- `locked` on a list checks that no new values appear, not that none were removed.
