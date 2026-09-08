# Media Custom Formats

[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**Custom format definitions, quality profiles, and release scoring for media automation.**

MuxCore sidecar module (`media.scoring`, `media.formats`) that stores custom formats and quality profiles in SQLite and scores releases over gRPC (`muxcore.formats.v1.FormatService` on `:9490` by default).

---

## How It Works

```
Release title/size/seeders ──→ media-custom-formats ──→ Score + quality parse
                                    │
                                    ├── Custom format rules (title/size/seeders)
                                    ├── Quality parse (resolution, source, codec, HDR)
                                    ├── Quality profile format scores
                                    └── Release profile groups (prefer / must / must-not)
```

### Key Features

- **Custom formats** — CRUD for named rule sets (`matches` / `contains` / numeric ops) with default scores
- **Quality profiles** — min/cutoff scores, upgrade settings, per-format score overrides
- **ScoreRelease / ParseQuality** — combine quality heuristics with format matches
- **Default seeds** — on empty DB, seeds Remux/HDR/x265/Proper/CAM formats and a Default Blocklist release group (enabled by default in the binary)
- **TRaSH / Recyclarr sync** — `SyncTrashGuides` imports the bundled official TRaSH catalog (WEB/Remux/Bluray tiers, HDR, x265, audio, LQ), a local Guides tree, or `guides_path=official` (hardcoded TRaSH-Guides GitHub zip — the Recyclarr-complete catalog). Household Settings → Quality and admin `/formats/sync-trash` call this RPC. Clients cannot supply a download URL.

### Default seeded custom formats

Inserted only when `custom_formats` is empty (`SeedDefaults` / `FORMATS_SEED_DEFAULTS=true` / binary default):

| ID | Name | Score | Rule |
|----|------|------:|------|
| `cf_seed_remux` | Remux | +100 | title `contains` `Remux` |
| `cf_seed_hdr` | HDR | +50 | title `matches` `(?i)\bHDR(10\|10\+|)?\b` |
| `cf_seed_x265` | x265/HEVC | +25 | title `matches` `(?i)x265\|h\.?265\|hevc` |
| `cf_seed_proper` | Proper/Repack | +20 | title `matches` `(?i)\b(proper\|repack)\b` |
| `cf_seed_cam` | CAM/TS | −10000 | title `matches` `(?i)\b(cam\|hdcam\|telesync\|hdts\|tc)\b` |

### Default seeded release profile group

| ID | Name | Behavior |
|----|------|----------|
| `rpg_seed_default` | Default Blocklist | `must_not_contain`: cam, telesync, hdcam; `preferred`: bluray, remux, web-dl (+15 preferred score) |

Admin UI: **Custom Formats** at `/formats`, quality profiles at `/formats/profiles` (admin-ui → media-custom-formats gRPC).

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `FORMATS_DB_PATH` | `/var/lib/media-custom-formats/formats.db` | SQLite database path |
| `FORMATS_GRPC_ADDR` | `:9490` | Module gRPC listen address |
| `FORMATS_SEED_DEFAULTS` | (see note) | Set to `true` to enable seeding when not already enabled via config |
| `FORMATS_TRASH_SYNC` | unset (off) | `true` imports TRaSH packs on start |
| `FORMATS_TRASH_IMPORT_PROFILES` | `true` when syncing from start | Also upsert quality profiles |
| `FORMATS_TRASH_SCORE_SET` | `default` | `trash_scores` key |
| `FORMATS_TRASH_SERVICES` | `radarr,sonarr` | Which guide trees to import |
| `FORMATS_TRASH_GUIDES_PATH` | (bundled fixture) | Local TRaSH-Guides root (`docs/json/…`), or `official` for the GitHub archive |
| `FORMATS_TRASH_OFFICIAL` | unset | When true on start sync, download the hardcoded TRaSH-Guides zip |
| `MUXCORE_MODULE_ID` | `media-custom-formats` | Module ID for mesh registration |
| `MUXCORE_INSECURE_DISABLE_TLS` | `` | Set to `true` for insecure mesh (dev only) |
| `MUXCORE_GRPC_ADDR` / `--muxcore-mesh-addr` | (SDK) | Core mesh address |

Note: `cmd/module` always starts with `SeedDefaults: true`, so the binary seeds an empty DB by default.

---

## Quick Start

```bash
make build

export MUXCORE_INSECURE_DISABLE_TLS=true
./media-custom-formats --muxcore-mesh-addr localhost:9090

# Example: parse quality from a title
grpcurl -plaintext -d '{"title": "Movie.2024.2160p.BluRay.REMUX.HDR.mkv"}' \
  localhost:9490 muxcore.formats.v1.FormatService/ParseQuality
```

---

## Development

```bash
make build
make test
make lint
make ci      # lint + test + build
make proto   # regenerate formatsv1 from proto/formatsv1/formats.proto
```

Requires a local `../core` checkout (`go.mod` replace directives).

---

## Compatibility

See [COMPATIBILITY.md](COMPATIBILITY.md). Module version `0.1.0`, min core `0.4.0`.

---

## License

GPL-3.0
