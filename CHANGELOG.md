# Changelog

## [0.1.9] — 2026-08-20

### Added
- Documented admin-ui sync button + manual/env cron options for TRaSH Guides (scheduler-cron not required).

## [0.1.8] — 2026-08-20

### Added
- **TRaSH Guides sync** — import all official custom formats (and optional quality profiles) from a local Guides tree or the GitHub archive.
- RPC `SyncTrashGuides`; settings `trash_sync_on_start`, `trash_guides_path`, `trash_score_set`, `trash_import_profiles`, `trash_sync_now`.
- Arr-compatible matching for TRaSH specs: release title/group, resolution, source, quality modifier; `regexp2` for lookaround patterns.
- Env: `FORMATS_TRASH_SYNC`, `FORMATS_TRASH_GUIDES_PATH`, `FORMATS_TRASH_GUIDES_URL`, `FORMATS_TRASH_CACHE_DIR`, `FORMATS_TRASH_SCORE_SET`, `FORMATS_TRASH_IMPORT_PROFILES`, `FORMATS_TRASH_SERVICES`.

## [0.1.7] — 2026-08-20

### Added
- Release profile CRUD RPCs: `ListReleaseProfiles`, `UpsertReleaseProfile`, `DeleteReleaseProfile`.

## [0.1.6] — 2026-08-10

### Added

- Advertise `settings` capability so admin-ui discovers SettingsProvider without ListAll probing.

## [0.1.5] — 2026-08-10

### Fixed
- Sync Info()/muxcore.json version to **0.1.5**.

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
