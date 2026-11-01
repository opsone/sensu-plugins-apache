# Changelog

## [0.2.0] - 2026-11-01

### Changed
- Rewritten in Go with the Sensu plugin SDK: static binary for `linux/amd64`, no Ruby runtime asset.
- Command renamed without the `.rb` suffix (`check-apache-status`).
- The `--host` shorthand is `-H` (`-h` is `--help`).
- `--host`, `--port` and `--path` are required, no more `127.0.0.1`, `80` and `apache-status` defaults in the plugin. Set them in the check definition.
- Every 3xx code, 401 and 403 are WARNING (endpoint misconfigured), like 301 and 404.
- Invalid arguments are UNKNOWN.

### Added
- The mod_status page (`?auto`) is parsed: busy workers compared with `--max-workers` (`MaxRequestWorkers`, scoreboard size by default), `--warning` (80 %) and `--critical` (95 %) thresholds.
- WARNING when the URL answers 200 with a page that is not a mod_status page.
- `--timeout` (default 10 s).
- Every option can be set through an environment variable (`CHECK_APACHE_*`).

## [0.1.1]

### Changed
- Update `check-apache-status.rb`.

## [0.1.0]

### Added
- `check-apache-status.rb`.
