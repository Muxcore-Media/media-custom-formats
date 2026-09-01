# Media Custom Formats

[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**Custom format definitions, quality profiles, TRaSH Guides sync, and release scoring for media automation.**

MuxCore sidecar module (`media.scoring`, `media.formats`) that stores custom formats and quality profiles in SQLite and scores releases over gRPC (`muxcore.formats.v1.FormatService` on `:9490` by default).

---

## How It Works

```
Release title/size/seeders ──→ media-custom-formats ──→ Score + quality parse
                                    │
                                    ├── Custom format rules (title/size/seeders + TRaSH specs)
                                    ├── Quality parse (resolution, source, codec, HDR)
                                    ├── Quality profile format scores
                                    └── Release profile groups (prefer / must / must-not)
```

### Key Features

- **Custom formats** — CRUD for named rule sets (`matches` / `contains` / numeric ops) with default scores
- **Quality profiles** — min/cutoff scores, upgrade settings, per-format score overrides
- **ScoreRelease / ParseQuality** — combine quality heuristics with format matches
- **TRaSH Guides sync** — pull the full official [TRaSH Guides](https://trash-guides.info/) custom-format (and optional quality-profile) catalog, the same source Recyclarr uses
- **Default seeds** — on empty DB (when trash sync-on-start is off), seeds Remux/HDR/x265/Proper/CAM formats and a Default Blocklist release group

### TRaSH Guides (recommended)

Best path for Arr-grade scoring: sync the Guides JSON tree into this module (upsert by `trash_id`).

```bash
# One-shot from a local clone (Recyclarr-compatible layout with metadata.json)
export FORMATS_TRASH_GUIDES_PATH=/path/to/TRaSH-Guides/Guides
grpcurl -plaintext -d '{"import_profiles":true}' \
  localhost:9490 muxcore.formats.v1.FormatService/SyncTrashGuides

# Or download the official GitHub archive on start
export FORMATS_TRASH_SYNC=true
export FORMATS_TRASH_IMPORT_PROFILES=true
# optional: FORMATS_TRASH_SCORE_SET=default
# optional: FORMATS_TRASH_SERVICES=radarr,sonarr
./media-custom-formats --muxcore-mesh-addr localhost:9090
```

`make trash-guides` clones/sparse-checks out Guides into `.cache/trash-guides` for offline sync.

When `ScoreRelease` includes `category`/`sub_category` (`movie` vs `tv`/`show`), Radarr vs Sonarr TRaSH formats are filtered so both catalogs can live in one DB without double-scoring.

### Default seeded custom formats

Inserted only when `custom_formats` is empty (`SeedDefaults` / `FORMATS_SEED_DEFAULTS=true` / binary default) **and** trash sync-on-start is not enabled:

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

Admin UI: **Custom Formats** at `/formats` (including **Sync TRaSH Guides**), quality profiles at `/formats/profiles` (admin-ui → media-custom-formats gRPC).

### Scheduled / automated sync

Built-in options:

1. **On start:** `FORMATS_TRASH_SYNC=true` (and optional `FORMATS_TRASH_IMPORT_PROFILES`, `FORMATS_TRASH_SCORE_SET`, `FORMATS_TRASH_SERVICES`)
2. **Interval:** `FORMATS_TRASH_INTERVAL=<hours>` runs an in-process ticker that refreshes stale cache and re-syncs
3. **Settings:** set `trash_sync_now=true` (or use admin Settings for the formats module)
4. **Admin UI:** Formats page → Sync TRaSH Guides
5. **External cron:** call the gRPC `SyncTrashGuides` RPC or re-trigger via settings

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `FORMATS_DB_PATH` | `/var/lib/media-custom-formats/formats.db` | SQLite database path |
| `FORMATS_GRPC_ADDR` | `:9490` | Module gRPC listen address |
| `FORMATS_SEED_DEFAULTS` | (see note) | Set to `true` to enable seeding when not already enabled via config |
| `FORMATS_TRASH_SYNC` | `` | `true` = sync TRaSH Guides on start |
| `FORMATS_TRASH_GUIDES_PATH` | `` | Local Guides root (`metadata.json`). Empty → download archive |
| `FORMATS_TRASH_GUIDES_URL` | GitHub `master.tar.gz` | Archive URL when path is empty |
| `FORMATS_TRASH_CACHE_DIR` | `/var/lib/media-custom-formats/trash-guides` | Download/extract cache |
| `FORMATS_TRASH_SCORE_SET` | `default` | `trash_scores` key (e.g. `german`, `anime-radarr`) |
| `FORMATS_TRASH_IMPORT_PROFILES` | `` | `true` = also upsert quality profiles |
| `FORMATS_TRASH_SERVICES` | `radarr,sonarr` | Comma list of services to import |
| `FORMATS_TRASH_INTERVAL` | `0` | Periodic sync interval in hours (`0` = disabled) |
| `MUXCORE_MODULE_ID` | `media-custom-formats` | Module ID for mesh registration |
| `MUXCORE_INSECURE_DISABLE_TLS` | `` | Set to `true` for insecure mesh (dev only) |
| `MUXCORE_GRPC_ADDR` / `--muxcore-mesh-addr` | (SDK) | Core mesh address |

Note: `cmd/module` always starts with `SeedDefaults: true`, so the binary seeds an empty DB by default unless `FORMATS_TRASH_SYNC=true` (then only the release-group seed runs).

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
make trash-guides  # sparse-clone official Guides into .cache/trash-guides
```

Requires Go 1.26+. Forgejo CI fetches `github.com/Muxcore-Media/*` modules from origin; local dev may use a sibling `../core` checkout via `go.work` or module cache.

---

## Compatibility

See [COMPATIBILITY.md](COMPATIBILITY.md). Module version `0.1.9`, min core `0.5.8`.

---

## License

GPL-3.0

TRaSH Guides content remains under its upstream license; this module only syncs and evaluates the published JSON definitions.
