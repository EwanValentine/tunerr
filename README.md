# tunerr

A small, containerised Go service that automates the **slskd → music library** pipeline.

It periodically scans a downloads directory and:

1. Sweeps failed-import batches back into the queue
2. Quarantines folders that contain no audio
3. Renames album folders to a consistent `Artist - Album (Year)` scheme using embedded audio tags
4. Moves tidied albums into the output library under `Artist/Year - Album/`
5. Optionally triggers a Lidarr rescan when files were moved

Runs are **idempotent**: re-running against the same state produces no unintended side-effects.

---

## How it works

```
DOWNLOAD_ROOT/
  complete/               ← albums land here from slskd
  failed_imports/
    complete/             ← slskd retry batches; swept back to complete/
    complete_1/
    incomplete_*/         ← left alone (still downloading)

OUTPUT_MUSIC_DIR/
  Pink Floyd/
    1973 - The Dark Side of the Moon/
      01 - Speak to Me.flac
      …
```

### Pipeline steps (each interval)

| Step | What happens |
|------|-------------|
| **0 – Sweep** | Every `complete*` sub-folder inside `failed_imports/` is merged back into `complete/`. Exact duplicates (same filename + size) are removed; size-mismatched files go to `_conflicts/`. |
| **1 – Park non-audio** | Folders in `complete/` that contain no audio files are moved to `complete/_non_audio/`. |
| **2 – Tidy** | Folders not already named `Artist - Album (YYYY)` are renamed using embedded tags (FLAC, MP3, M4A, OGG, OPUS, WAV, AIFF). If tags are missing or unreadable, the folder is left untouched and a warning is logged. |
| **3 – Move** | Well-named folders are moved to `OUTPUT_MUSIC_DIR/<Artist>/<YYYY> - <Album>/`. Cross-device mounts are handled transparently via copy+delete. |
| **4 – Lidarr** | If `LIDARR_RESCAN=true` and files were moved this run, `POST /api/v1/command {"name":"RescanFolders"}` is called. |

### Conflict rules

| Situation | Action |
|-----------|--------|
| Same filename, same size | Source removed (duplicate) |
| Same filename, different size | Source moved to `_conflicts/<name>` inside destination folder |

### No renames without data

If a folder has no audio files, or if the first audio file's tags lack both `artist` and `album`, the folder is **not renamed**. It stays in `complete/` with a log warning.

---

## Configuration

All configuration is via environment variables. **No API keys or paths are hardcoded.**

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DOWNLOAD_ROOT` | ✓ | — | Root of slskd downloads (contains `complete/`, `failed_imports/`) |
| `OUTPUT_MUSIC_DIR` | ✓ | — | Root of your music library |
| `COMPLETE_DIR` | | `$DOWNLOAD_ROOT/complete` | Override the complete folder path |
| `FAILED_IMPORTS_DIR` | | `$DOWNLOAD_ROOT/failed_imports` | Override the failed imports path |
| `INTERVAL_SECONDS` | | `300` | How often the pipeline runs |
| `DRY_RUN` | | `false` | Set to `true` to log all actions without touching any files |
| `LOG_PATH` | | stdout | Write logs to this file instead of stdout |
| `LIDARR_URL` | | — | Base URL of your Lidarr instance, e.g. `http://lidarr:8686` |
| `LIDARR_API_KEY` | | — | Lidarr API key |
| `LIDARR_RESCAN` | | `false` | Set to `true` to trigger a rescan after runs where files moved |
| `MB_ENABLED` | | `false` | Set to `true` to look up missing release years from MusicBrainz |
| `MB_USER_AGENT` | | `tunerr/1.0 (…)` | User-Agent sent to MB API — MusicBrainz requires a meaningful value with contact info |

---

## Running

### docker run

```sh
docker build -t tunerr .

docker run -d \
  --name tunerr \
  --restart unless-stopped \
  -e DOWNLOAD_ROOT=/downloads/slskd \
  -e OUTPUT_MUSIC_DIR=/music \
  -e INTERVAL_SECONDS=300 \
  -e DRY_RUN=false \
  -e LIDARR_URL=http://lidarr:8686 \
  -e LIDARR_API_KEY=your_api_key_here \
  -e LIDARR_RESCAN=true \
  -v /path/to/slskd/downloads:/downloads/slskd \
  -v /path/to/music:/music \
  tunerr
```

### docker compose

Copy `compose.yml`, fill in your volume paths, and run:

```sh
docker compose up -d
```

Dry-run first (recommended before first use):

```sh
DRY_RUN=true docker compose up
```

### Local (no Docker)

```sh
export DOWNLOAD_ROOT=/tmp/slskd
export OUTPUT_MUSIC_DIR=/tmp/music
export DRY_RUN=true
go run .
```

---

## Logs

Logs are structured JSON (via stdlib `log/slog`). Each run ends with a `SUMMARY` line:

```json
{
  "time": "2026-03-19T12:00:05.123Z",
  "level": "INFO",
  "msg": "SUMMARY",
  "duration_ms": 412,
  "movedFolders": 3,
  "movedFiles": 47,
  "duplicateFiles": 2,
  "conflictFiles": 0,
  "nonAudioMoved": 1,
  "failedImportsSwept": 1,
  "errors": 0
}
```

Pipe through `jq` for human-readable output:

```sh
docker logs -f tunerr | jq -r '[.time, .level, .msg] | join(" | ")'
```

---

## MusicBrainz year lookup

Enable with `MB_ENABLED=true`. When active, any album whose audio tags have no year will be looked up against the [MusicBrainz release-group API](https://musicbrainz.org/doc/MusicBrainz_API). The year is extracted from the `first-release-date` field.

- Results are cached in-memory (per process lifetime) so each `(artist, album)` pair hits the network at most once.
- The client enforces MusicBrainz's 1 request/second rate limit automatically.
- MB lookups are purely additive: they never override a year already present in the audio tags.
- Set `MB_USER_AGENT` to something identifying — MusicBrainz blocks generic User-Agents.

### Running live tests

```sh
MB_LIVE=1 go test ./... -run TestMBClientLive -v
```

### Extending further

`YearLookup` in `tidy.go` is the interface:

```go
type YearLookup interface {
    LookupYear(artist, album string) (int, error)
}
```

Swap in any implementation (e.g. a local database, Discogs API) by passing it to `tidyAlbumFoldersWithLookup`.

---

## Safety guarantees

- **No silent deletes.** The only files removed are exact duplicates (same filename + same byte-count). Every removal is logged.
- **No overlapping runs.** A `sync.Mutex` with `TryLock` ensures a slow run does not cause the next tick to start on top of in-flight work.
- **Idempotent.** Running twice on the same state is safe: well-named folders already in the output are merged (with conflict protection) or skipped.
- **Internal dirs ignored.** Any folder starting with `_` (`_non_audio`, `_conflicts`) is never treated as an album.
