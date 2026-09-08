# TRaSH Guides fixture source

`official-*.json` files are vendored from the public
[TRaSH-Guides/Guides](https://github.com/TRaSH-Guides/Guides) repository
(`docs/json/{radarr,sonarr}/cf` and `quality-profiles`) so household installs
can sync Recyclarr-compatible custom formats offline.

Upstream: https://github.com/TRaSH-Guides/Guides
Fetched: 2026-09-08 (`refs/heads/master`)

Hand-authored `*.json` files (without the `official-` prefix) stay as extra
title-regex formats for remux/HDR/CAM scoring when a release has no known group.

LanguageSpecification / IndexerFlagSpecification CFs (Language: Not English,
FreeLeech) are title-token approximations. English+negate matches a positive
non-English language token so typical English releases without `EN` stay
score-neutral.
