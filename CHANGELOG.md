# Changelog

## [v0.0.15](https://github.com/tokuhirom/db-schema-sync/compare/v0.0.14...v0.0.15) - 2026-01-20
- chore: update flake.nix to v0.0.14 by @tokuhirom-tagpr[bot] in https://github.com/tokuhirom/db-schema-sync/pull/37
- docs: enhance README with comprehensive workflow examples and CI/CD integration by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/39

## [v0.0.14](https://github.com/tokuhirom/db-schema-sync/compare/v0.0.13...v0.0.14) - 2026-01-20
- feat: add Nix flake support with automated hash updates by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/35

## [v0.0.13](https://github.com/tokuhirom/db-schema-sync/compare/v0.0.12...v0.0.13) - 2026-01-19
- feat: add dry-run output to on-before-apply hook by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/33

## [v0.0.12](https://github.com/tokuhirom/db-schema-sync/compare/v0.0.11...v0.0.12) - 2026-01-19
- feat: add PostgreSQL advisory lock for concurrent apply protection by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/28

## [v0.0.11](https://github.com/tokuhirom/db-schema-sync/compare/v0.0.10...v0.0.11) - 2026-01-19
- docs: add CLAUDE.md for Claude Code guidance by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/29
- feat: capture psqldef output and pass to on-apply-failed hook by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/31

## [v0.0.10](https://github.com/tokuhirom/db-schema-sync/compare/v0.0.9...v0.0.10) - 2026-01-19
- test: add comprehensive unit tests for S3 operations and hooks by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/26

## [v0.0.9](https://github.com/tokuhirom/db-schema-sync/compare/v0.0.8...v0.0.9) - 2026-01-19
- docs: update README for new features by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/23
- feat: add environment variables to lifecycle hooks by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/25

## [v0.0.8](https://github.com/tokuhirom/db-schema-sync/compare/v0.0.7...v0.0.8) - 2026-01-19
- docs: add AWS environment variables to help output by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/17
- feat: add --db flag to diff command for database comparison by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/19
- feat: add fetch subcommand to download latest completed schema by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/20
- feat: add --export-after-apply flag to export schema after successful apply by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/22
- feat: add plan command for offline schema comparison by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/21

## [v0.0.7](https://github.com/tokuhirom/db-schema-sync/compare/v0.0.6...v0.0.7) - 2026-01-19
- fix: use correct flag names for S3 options by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/15

## [v0.0.6](https://github.com/tokuhirom/db-schema-sync/compare/v0.0.5...v0.0.6) - 2026-01-19
- feat: add on-before-apply lifecycle hook by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/12
- feat: add Prometheus metrics endpoint to watch command by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/14

## [v0.0.5](https://github.com/tokuhirom/db-schema-sync/compare/v0.0.4...v0.0.5) - 2026-01-17
- feat: add subcommand structure (watch, apply, diff) by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/8

## [v0.0.4](https://github.com/tokuhirom/db-schema-sync/compare/v0.0.3...v0.0.4) - 2026-01-16
- chore: move Docker build to goreleaser by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/9

## [v0.0.3](https://github.com/tokuhirom/db-schema-sync/compare/v0.0.2...v0.0.3) - 2026-01-16
- feat: use semver for version comparison by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/5
- feat: add custom S3 endpoint support for S3-compatible storage by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/7

## [v0.0.2](https://github.com/tokuhirom/db-schema-sync/compare/v0.0.1...v0.0.2) - 2026-01-16
- fix: update goreleaser main path to match new directory structure by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/3

## [v0.0.1](https://github.com/tokuhirom/db-schema-sync/commits/v0.0.1) - 2026-01-16
- use golangci-lint-action@v9 by @tokuhirom in https://github.com/tokuhirom/db-schema-sync/pull/1
