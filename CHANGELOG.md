# Changelog

## [0.1.15] - 2026-10-05


### Security
- gRPC server and peer dials use mesh TLS (meshtls, sdk/go/module v0.6.5) unless the dev insecure flag is set (ADR-0016/0017).

## [0.1.14] - 2026-10-05

### Changed
- Built on core v0.6.14 / sdk/go/module v0.6.4: unregisters on shutdown and re-registers after core restarts (ADR-0022).

## [0.1.11] — 2026-09-08

### Added
- `SyncTrashGuides` `guides_path=official` downloads the hardcoded TRaSH-Guides GitHub archive into the module data dir (Recyclarr-complete catalog). Tests use a fixture zip; clients cannot supply a URL.
- `FORMATS_TRASH_OFFICIAL=true` uses that official zip on start sync.

## [0.1.10] — 2026-09-08

### Added
- TRaSH `LanguageSpecification` and `IndexerFlagSpecification` map to title tokens so Language: Not English and FreeLeech score from the release name.
- Official `language-not-english` and `freeleech` custom formats in the bundled offline pack.

## [0.1.9] — 2026-09-08

### Added
- `ListReleaseProfiles` / `UpsertReleaseProfile` / `DeleteReleaseProfile` so household Settings can edit must-contain / must-not-contain / preferred terms (Radarr/Sonarr release profiles). Scoring already applied these groups.

## [0.1.8] — 2026-09-08

### Added
- Official TRaSH Guides custom formats (WEB/Remux/Bluray tiers, HDR, x265, audio, LQ) vendored offline.
- Parser maps ReleaseGroup, Source, Resolution, and QualityModifier specs so Recyclarr JSON imports.

## [0.1.7] — 2026-09-08

### Added

- `SyncTrashGuides` RPC: import Recyclarr-compatible TRaSH custom formats and quality profiles.
- Bundled HD/UHD fixture pack (offline-safe). Optional `FORMATS_TRASH_GUIDES_PATH` for a local Guides clone.
- `FORMATS_TRASH_SYNC=true` on start imports the pack (run-host / compose default).

## [0.1.6] — 2026-08-10

### Added

- Advertise `settings` capability so admin-ui discovers SettingsProvider without ListAll probing.

## [0.1.5] — 2026-08-10

### Fixed
- Sync Info()/muxcore.json version to **0.1.5**.

## [0.1.13] - 2026-10-05


### Changed
- Reported version comes from muxcore.json (ADR-0021); built on core v0.6.12 / sdk/go/module v0.6.3 (mesh enrollment, ADR-0017).

## [0.1.11] - 2026-10-05

### Changed
- CI runs on GitHub-hosted runners from the umbrella template; retired-origin workflows removed.
- Dependencies resolve from published GitHub tags (no filesystem `replace`); requires core v0.6.0.

## [0.1.4] — 2026-08-10

### Added
- `SettingsProvider` mesh wiring (`seed_defaults` / `FORMATS_SEED_DEFAULTS`) via `RegisterSettings`.

### Fixed
- Module `Info().Version` aligned to **0.1.4** (was stuck at 0.1.0).

## [0.1.3]

### Added
- Documented default seeded custom formats / release group in README.
- `TestSeedDefaults` asserts Remux/HDR/x265/Proper/CAM + Default Blocklist seeds.
- MVP `smoke.sh` covers admin-ui `GET /formats` (seeded Remux or Custom Formats shell).

## [0.1.2]

- Prior release pin (core@v0.5.0).
