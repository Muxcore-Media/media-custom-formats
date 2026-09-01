# Compatibility

## Core Version

| Module Version | Core Version | Status |
|----------------|-------------|--------|
| v0.1.9         | v0.5.8+     | Current |
| v0.1.8         | v0.5.0+     | Compatible |
| v0.1.2         | v0.5.0+     | Superseded |
| v0.1.0         | v0.4.0+     | Superseded |

## Contracts / capabilities

| Capability | Status |
|------------|--------|
| `media.scoring` | Current |
| `media.formats` | Current |
| `settings` | Current |

gRPC: `muxcore.formats.v1.FormatService` (default `:9490`), including `SyncTrashGuides`.

Default seeds (empty DB only, when TRaSH sync did not populate formats): see README “Default seeded custom formats”. TRaSH Guides: see README “TRaSH Guides (recommended)”. Admin pages: `/formats`, `/formats/profiles`.
