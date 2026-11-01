# sensu-plugins-apache

Sensu Go check reading the Apache [mod_status](https://httpd.apache.org/docs/2.4/mod/mod_status.html) page:

- `check-apache-status`: Apache answers on its status page, and its busy workers stay below the saturation thresholds.

Static Go binary (no CGO, no Ruby runtime): one asset for every Linux distribution, `amd64` only.

## Behavior

The check requests `http://<host>:<port>/<path>?auto` (the machine-readable status page), without following redirections, and compares `BusyWorkers` with the server capacity.

The capacity is `--max-workers`, i.e. the `MaxRequestWorkers` of the server (150 by default on Debian, whatever the MPM). The status page does not give it: with `--max-workers 0`, the scoreboard size is used instead, which is `ServerLimit` × `ThreadsPerChild` and may be much larger (400 slots on Debian for 150 usable workers), hence an underestimated saturation.

| Situation | State |
|---|---|
| Busy workers below `--warning` % of the capacity | OK |
| Busy workers ≥ `--warning` % (default 80) | WARNING |
| Busy workers ≥ `--critical` % (default 95) | CRITICAL |
| HTTP 3xx, 401, 403, 404: status endpoint misconfigured | WARNING |
| HTTP 200 on a page that is not a status page (no `BusyWorkers`) | WARNING |
| Other HTTP code (5xx…) | CRITICAL |
| Connection refused, timeout | CRITICAL |
| Invalid argument | UNKNOWN |

Output example:

```
check-apache-status OK: Apache/2.4.69 (Unix) event MPM: 12/150 workers busy (8%)
```

Apache side, the status handler must be reachable from the agent, e.g.:

```apache
<Location /apache-status>
    SetHandler server-status
    Require local
</Location>
```

`ExtendedStatus` is not needed.

## Usage

```
check-apache-status [flags]
  -H, --host string       Apache host, e.g. 127.0.0.1 (required)
  -p, --port int          Apache port, e.g. 80 (required)
  -q, --path string       Path of the mod_status handler, e.g. apache-status (required, ?auto is appended)
  -m, --max-workers int   MaxRequestWorkers of the server, used as capacity (0: scoreboard size, which may overestimate it)
  -w, --warning int       Busy workers percentage of the capacity raising a warning (default 80)
  -c, --critical int      Busy workers percentage of the capacity raising a critical (default 95)
      --timeout int       HTTP request timeout in seconds (default 10)
```

Every option can also be set with an environment variable:

| Option | Variable |
|---|---|
| `--host` | `CHECK_APACHE_HOST` |
| `--port` | `CHECK_APACHE_PORT` |
| `--path` | `CHECK_APACHE_PATH` |
| `--max-workers` | `CHECK_APACHE_MAX_WORKERS` |
| `--warning` / `--critical` | `CHECK_APACHE_WARNING` / `CHECK_APACHE_CRITICAL` |
| `--timeout` | `CHECK_APACHE_TIMEOUT` |

Example:

```sh
check-apache-status -H 127.0.0.1 -p 80 -q server-status -m 400
```

## Check definition

Every option is passed through `env_vars`, filled per host from entity annotations with token substitution (applied by the agent to the whole check definition, `env_vars` included). One annotation maps to one option.

Naming rule: an annotation is named after its environment variable, lowercased, without the `CHECK_` prefix (`CHECK_APACHE_MAX_WORKERS` → `apache_max_workers`).

Every annotation is optional: the check definition holds the defaults. An empty variable is ignored and the option keeps its default value.

Annotations, e.g. in the agent configuration (`/etc/sensu/agent.yml`):

```yaml
annotations:
  apache_host: 127.0.0.1          # default 127.0.0.1
  apache_port: "8080"             # default 80
  apache_path: server-status      # default apache-status
  apache_max_workers: "400"       # MaxRequestWorkers, default 150
  apache_warning: "70"            # default 80
  apache_critical: "90"           # default 95
```

```yaml
---
type: CheckConfig
api_version: core/v2
metadata:
  name: check-apache-status
spec:
  command: check-apache-status
  env_vars:
    - CHECK_APACHE_HOST={{ .annotations.apache_host | default "127.0.0.1" }}
    - CHECK_APACHE_PORT={{ .annotations.apache_port | default "80" }}
    - CHECK_APACHE_PATH={{ .annotations.apache_path | default "apache-status" }}
    - CHECK_APACHE_MAX_WORKERS={{ .annotations.apache_max_workers | default "150" }}
    - CHECK_APACHE_WARNING={{ .annotations.apache_warning | default "" }}
    - CHECK_APACHE_CRITICAL={{ .annotations.apache_critical | default "" }}
    - CHECK_APACHE_TIMEOUT={{ .annotations.apache_timeout | default "" }}
  runtime_assets:
    - opsone/sensu-plugins-apache
  interval: 60
  timeout: 30
  publish: true
  subscriptions:
    - apache
```

## Migrating from the Ruby version (≤ 0.1.1)

- The command loses its `.rb` suffix: `check-apache-status.rb` → `check-apache-status`.
- The host shorthand is now `-H` (`-h` is `--help`).
- `--host`, `--port` and `--path` are required (they defaulted to `127.0.0.1`, `80` and `apache-status`); the defaults now live in the check definition.
- The status page content is read: a page that is not a mod_status page is a WARNING, and worker saturation raises WARNING / CRITICAL. Set `apache_max_workers` to the `MaxRequestWorkers` of each host not using the Debian default (150).
- Every 3xx code, 401 and 403 are WARNING like 301 and 404 (they were CRITICAL).
- Request timeout of 10 s (`--timeout`), instead of the 60 s Ruby defaults.
- Invalid arguments are UNKNOWN.
- The asset no longer depends on the Ruby runtime asset, nor on the Debian release.

## Build and release

```sh
make test          # go vet + unit tests
make integration   # runs the check against a real Apache in Docker
```

Release:

```sh
git tag 0.2.0 && git push origin 0.2.0
make release       # version taken from the tag, or forced: make release VERSION=0.2.0
sensuctl create -f dist/opsone-sensu-plugins-apache_0.2.0.yml
```

`make release` runs `make dist`, which runs the tests then produces in `dist/`:

- one archive per architecture listed in `ARCHS` (`linux/amd64`) in Sensu asset layout (`bin/check-apache-status`);
- the sha512 checksums file;
- `opsone-sensu-plugins-apache_<version>.yml`, the asset definition pointing to the GitHub release of the tag.

It then creates the GitHub release of the tag with `gh`, uploads the archive and checksums, and uses the asset definition as release description.

`check-apache-status version` prints the version, commit and build date.
