# Randamu

A single-binary, idempotent image fetcher. It periodically downloads images
from pixiv (bookmarks, tag searches, the recommendation feed) and from the
unauthenticated lolicon / konachan / danbooru sources into a local directory,
so a wallpaper tool can simply pick a random file from disk.

This replaces the old Python/FastAPI random-wallpaper service.

## Features

- Sources: pixiv public/private bookmarks, tag search and the recommended
  feed; plus lolicon, konachan and danbooru (no pixiv token needed for those)
- Idempotent: files are named after the source and work id, so re-runs skip
  everything already downloaded
- WebP conversion on by default: downloads are transcoded to WebP to save
  disk space (configurable quality, can be disabled)
- Source metadata is embedded inside each image file (JPEG COM, PNG tEXt,
  WebP XMP) so files are recognisable as randamu output
- Reliable downloads: bounded concurrency, retries with backoff, `Range` resume for partial files
- Optional proxy support (`http`, `https`, `socks5`)
- Single self-contained binary (libwebp is statically linked in), low memory
- systemd timer integration through the bundled NixOS and home-manager modules
- Filters for ugoira, AI-generated works, R18/R18G, blacklisted tags and page count
- Content analysis rejects non-illustration images: QR-code posters, mosaic-censored works and fully blurred paid-site teasers
- Multi-page variations (差分) are merged into looping animated PNGs (APNG), one per variation set

## Requirements

- Go 1.25+ and a C compiler with libwebp headers (or Nix). These are only
  needed to **build** randamu — the released binaries already contain libwebp
  and need nothing at runtime.
- A pixiv refresh token — **only** if you use pixiv sources; lolicon,
  konachan and danbooru need no authentication
- Optional: a proxy if pixiv is unreachable from your network
- Optional: `tesseract` for the promo-text check. The Windows x64/x86 release
  archives already include it under `tesseract/`; the Windows arm64 archive and
  the Linux/macOS archives look for it in `PATH`

## Install

- **Debian/Ubuntu** — `sudo dpkg -i randamu_*_linux_x64.deb` (or `apt install ./...`),
  installs `/usr/bin/randamu` plus `randamu.service` and `randamu.timer`; enable
  the schedule with `sudo systemctl enable --now randamu.timer`. Create the
  service user first: `sudo useradd --system --home-dir /var/lib/randamu --create-home randamu`
  (the units assume user/group `randamu`).
- **Fedora/RHEL** — `sudo rpm -i randamu_*_linux_x64.rpm`, same units and setup.
- **Arch (AUR)** — not published; build from source or use the tar.gz.
- **Windows** — run `randamu_*_windows_x64_setup.exe` (a per-architecture NSIS
  installer that also adds `randamu` to `PATH` and bundles tesseract for
  x64/x86), or use the zip with `install.ps1` to register a daily scheduled
  task. `uninstall.ps1` removes the task; the installer has its own uninstaller.
  The scripts and their options are documented in `contrib/windows/README.md`.
- **macOS** — `sudo installer -pkg randamu_*_macos_aarch64.pkg -target /`
  installs `/usr/local/bin/randamu`; the archive is unsigned, so a downloaded
  pkg may need `xattr -d com.apple.quarantine <file>.pkg` first. The tar.gz
  works without an installer.
- **Nix** — `nix profile install github:MaikoTan/randamu`, or use the bundled
  NixOS / home-manager modules (below).
- **Any other Linux** — unpack the tar.gz and schedule it yourself; reference
  systemd units live in `contrib/systemd/`.

## Getting a refresh token

Follow [this guide](https://gist.github.com/ZipFile/c9ebedb224406f4f11845ab700124362).
Never commit the token to the repository — pass it through the environment or a
systemd credential.

## Build

```bash
nix build            # ./result/bin/randamu（Linux 上是完全静态的 musl 二进制）
# 或，本机已装 libwebp 开发包：
CGO_ENABLED=1 go build -trimpath -o randamu .
```

WebP 编解码通过 cgo 调用 libwebp。发布产物与 `nix build` 会用 zig 把
libwebp 编成**静态库**再链接进二进制（`flake.nix` 与 release 流水线里各
有一份同样的交叉编译逻辑），因此二进制仍是单个可执行文件，不依赖系统
安装的 libwebp；Linux 产物还是完全静态的，glibc 与 musl 发行版通用。
用 `CGO_ENABLED=0` 构建也能通过，但 WebP 转换会被跳过并保留原图。

## Usage

```bash
# pixiv 图源需要 token；仅用 lolicon/konachan/danbooru 时可省略
export PIXIV_REFRESH_TOKEN="..."
randamu --config examples/config.json
```

A one-shot run downloads at most `limit` works per source and exits. Schedule it
with cron, a systemd timer (see below), or run it manually.

### Configuration

Configuration is JSON. CLI flags and environment variables override the file,
and the file overrides built-in defaults:

```json
{
  "images_dir": "~/.local/share/randamu/images",
  "state_dir": "~/.local/state/randamu",
  "proxy": "",
  "concurrency": 4,
  "max_retries": 3,
  "max_page_count": 10,
  "skip_ai": true,
  "skip_r18": true,
  "skip_r18g": true,
  "skip_qr": true,
  "skip_mosaic": true,
  "skip_blur": true,
  "skip_promo_text": true,
  "qr_min_coverage": 0.02,
  "merge_variations": true,
  "variation_min_similarity": 0.9,
  "variation_delay_ms": 1000,
  "variation_match_character": true,
  "max_size_mb": 0,
  "max_age_days": 0,
  "webp": true,
  "webp_quality": 85,
  "blacklist_tags": ["漫画", "講座", "描き方", "MMD", "3D", "3Dカスタム少女"],
  "sources": [
    { "type": "bookmarks", "restrict": "public", "limit": 100 },
    { "type": "bookmarks", "restrict": "private", "limit": 100 },
    {
      "type": "search",
      "tag": "拘束",
      "search_target": "exact_match_for_tags",
      "sort": "popular_desc",
      "limit": 100
    },
    { "type": "recommended", "limit": 100 },
    { "type": "lolicon", "tags": ["風景"], "rating": "safe", "limit": 20 },
    { "type": "konachan", "tags": ["scenic"], "rating": "safe", "limit": 20 },
    { "type": "danbooru", "tags": ["landscape"], "rating": "safe", "limit": 20 }
  ]
}
```

### Sources

Each entry in `sources` has a `type`:

- `bookmarks` (`restrict`: `public` / `private`), `search` (`tag`, `search_target`, `sort`, `duration`),
  `recommended` — pixiv sources; **require** a pixiv refresh token.
- `lolicon` — random works from the lolicon v2 API (`tags`, `rating`).
- `konachan`, `danbooru` — tag search / newest / random (`tags`, `rating`, `random`).

The non-pixiv sources accept extra fields:

| Field | Meaning |
|---|---|
| `tags` | tag list (konachan/danbooru join them with spaces) |
| `rating` | `safe`, `questionable` or `explicit` |
| `random` | pick a random page instead of paging forward |
| `api_base` | override the API root (mirrors); image host allowlist is then relaxed |

If any selected source is a pixiv type, a pixiv token is required; otherwise
randamu runs without one.

Config path resolution: `--config` > `$RANDAMU_CONFIG` > `$XDG_CONFIG_HOME/randamu/config.json`.

Environment variables: `PIXIV_REFRESH_TOKEN`, `RANDAMU_CONFIG`, `RANDAMU_IMAGES_DIR`,
`RANDAMU_STATE_DIR`, `RANDAMU_PROXY`, `RANDAMU_WEBP`.

### Content analysis

The `skip_qr`, `skip_mosaic`, `skip_blur` and `skip_promo_text` options (all
on by default) reject images that are not usable illustrations. The pixel
checks are pure Go; the optional promo-text check shells out to `tesseract`:

- **QR poster** — scans for QR finder patterns (the 1:1:3:1:1 corner squares)
  at a higher resolution than the other checks and rejects the image when the
  code covers at least `qr_min_coverage` (2% by default). Promo pages often
  put a small QR in a corner.
- **Mosaic censorship** — rejects the image when more than half of its
  16×16 blocks are flat and adjacent flat blocks alternate sharply, the
  checkerboard signature of pixelated censorship. A plain sky or a smooth
  gradient is not affected.
- **Full-image blur** — rejects the image when almost no pixel carries a
  sharp edge, the signature of a teaser that only exists to push viewers to
  a paid site (Fanbox, Fantia, …).
- **Promo text** — with `skip_promo_text` (on by default), OCRs the image
  with the optional `tesseract` binary and rejects pages whose overlaid text
  mentions Fanbox, Fantia, Patreon, DLsite, Gumroad, BOOTH, ci-en, Ko-fi or
  Afdian. The Windows x64 and x86 release archives bundle tesseract, so the
  check works there without installing anything. Elsewhere, when `tesseract`
  is not in `PATH` (and not bundled next to the executable) the check is
  skipped and the image is kept; add it with
  `services.randamu.extraPackages = [ pkgs.tesseract ];`.

Analysis runs once per downloaded file and never re-downloads: rejected
illust ids are remembered in `state.json` and skipped on later runs. Analysis
runs on the original download, before WebP conversion; undecodable formats are
kept unchanged.

### Variation merging (差分)

With `merge_variations` enabled (the default), the pages of one work are
grouped into variation sets and each set is merged into a looping APNG
(`variation_delay_ms` apart, 1000 ms by default), removing the pages it
merged. Consecutive pages form a set while:

- every page decoded successfully,
- the pages share the same pixel size, and
- each page resembles the first page of its set: either the overall luma
  correlation is at least `variation_min_similarity` (0.9 by default, range
  0–1), or the central half correlates at least 0.97, which covers variations
  that repaint the background or change the framing while the subject stays in
  place.

With `variation_match_character` enabled (the default), a page is also
accepted when the main character keeps the same clothing and hair colors in
the same place. It compares the saturated colors of the central subject and
their spatial distribution, so it still works when the face is hidden;
black-and-white pages have too little color for this check.

When every page belongs to one set the APNG is `{id}.png`. When a work mixes
several sets with unrelated pages — an artist uploading separate commissions
in one post — each set becomes `{id}_v1.png`, `{id}_v2.png`, … and the pages
that resemble nothing keep their page files untouched. Works that contain no
variation at all, such as ordinary multi-page manga, are left alone.

The thresholds are normalized cross-correlations, so variations that recolor
or restyle parts of the picture still merge, while sequential manga pages do
not.

A merged APNG is detected on later runs and skipped, so its pages are never
downloaded again. APNG stores every frame at full resolution, so the merged
file is roughly the size of all its pages together. WebP pages cannot be
decoded by the Go standard library and are never merged.

### Retention

Downloads are otherwise never deleted, so the image directory grows with every
run. Two optional limits prune it after each run (both off by default):

- `max_size_mb` — when the downloaded files exceed this many MiB, the oldest
  files are deleted first until the directory fits.
- `max_age_days` — downloaded files older than this many days are deleted.

Only files randamu created are considered, recognised by their names
(`{id}_p{n}.{ext}`, `{source}_{id}_p{n}.{ext}`, merged `{id}.png` /
`{id}_v{n}.png`); anything else in the directory is left alone. Orphan `.part`
files left by an interrupted download that are older than 24 hours are removed
whenever either limit is set.

Files that belong to a work still seen in a source may be downloaded again on
a later run once retention removed them.

### WebP conversion

With `webp` enabled (the default), every downloaded JPEG or PNG is transcoded
to WebP at `webp_quality` (default 85) before it is stored, and the file
extension becomes `.webp`. This keeps the image directory small. GIF files
(possibly animated) and files that are already WebP are stored unchanged. If a
transcode fails the original file is kept and the run continues.

Because WebP output cannot be decoded by the Go standard library, variation
merging is skipped when `webp` is on, and content analysis runs on the original
download before conversion. Disable WebP (`webp: false` or `--webp off`) if you
need APNG variation merging.

### Embedded source metadata

Every stored file carries a compact `randamu:v1:{json}` payload identifying its
source, work id, page, title, author, tags, original URL and download time. It
is written into the image container without touching pixel data:

- **JPEG** — a `COM` segment,
- **PNG** — a `tEXt` chunk with keyword `Randamu`,
- **WebP** — an XMP chunk (fourcc `XMP` + trailing space).

The metadata both marks a file as randamu output and lets a future management
tool recover where each image came from.

### Token sources

Only pixiv sources need a token. Tried in order:

Tried in order:

1. `--refresh-token-file <path>`
2. `PIXIV_REFRESH_TOKEN`
3. `<state_dir>/refresh_token` (rotated token from a previous run)
4. `$CREDENTIALS_DIRECTORY/pixiv_refresh_token` (systemd `LoadCredential`)

If pixiv rotates the refresh token, the new value is written atomically to the
state directory with mode `0600`.

### CLI

```
--config PATH               config file
--images-dir PATH           override download directory
--state-dir PATH            override state directory
--proxy URL                 proxy for API and image requests
--concurrency N             parallel downloads
--refresh-token-file PATH   file containing the refresh token
--source TYPE               only run this source type (repeatable)
--webp on|off               convert downloads to WebP (default on)
--webp-quality N            WebP quality 1-100 (default 85)
--dry-run                   list what would be downloaded or deleted, write nothing
--log-level LEVEL           debug|info|warn|error
--version
```

Exit codes: `0` success (including skips), `1` fatal, `2` invalid config,
`3` authentication failure (pixiv sources only), `4` one or more downloads
failed after retries.

## Idempotency

- pixiv files are named `{illust_id}_p{n}.{ext}`; other sources prefix the
  source name (`{source}_{id}_p{n}.{ext}`) so numeric ids from different
  sources do not collide. Existing files are skipped with a single `stat`.
  Files downloaded by the old fish/aria2c script are recognized.
- When WebP conversion is on, an existing `.webp` file also counts as present,
  so a work is not downloaded twice under different extensions.
- Downloads are written to `<name>.part` and atomically renamed on success.
  Interrupted transfers resume with an HTTP `Range` request on the next run.
- Per-source cursors live in `<state_dir>/state.json`; a page's cursor only
  advances after every job from that page completed or was permanently skipped.
- State loss is safe: downloads are deduplicated by filename.

### State directory

| File | Purpose |
|---|---|
| `state.json` | pagination cursors, recommended offset, recently viewed ids, rejected illust ids |
| `refresh_token` | rotated refresh token (mode `0600`) |
| `lock` | prevents overlapping runs |

## NixOS module

```nix
{
  inputs.randamu.url = "github:MaikoTan/randamu";

  # in a NixOS configuration:
  imports = [ inputs.randamu.nixosModules.default ];

  sops.secrets."pixiv/refresh_token" = { };

  services.randamu = {
    enable = true;
    refreshTokenFile = config.sops.secrets."pixiv/refresh_token".path;
    settings = {
      sources = [
        { type = "bookmarks"; restrict = "public"; limit = 100; }
        { type = "recommended"; limit = 100; }
      ];
    };
  };
}
```

Defaults: user/group `randamu`, images in `/var/lib/randamu/images`, state in
`/var/lib/randamu`, timer runs `daily` with a randomized delay of one hour.

## home-manager module

```nix
{
  imports = [ inputs.randamu.homeManagerModules.default ];

  sops.secrets."pixiv/refresh_token" = { };

  services.randamu = {
    enable = true;
    refreshTokenFile = config.sops.secrets."pixiv/refresh_token".path;
    settings.sources = [
      { type = "bookmarks"; restrict = "public"; limit = 100; }
      { type = "recommended"; limit = 100; }
    ];
  };
}
```

Defaults: images in `$XDG_DATA_HOME/randamu/images`, state in
`$XDG_STATE_HOME/randamu`, user-level systemd timer running `daily`.

Common module options: `imagesDir`, `stateDir`, `configFile`, `settings`,
`refreshTokenFile`, `environmentFile`, `proxy`, `onCalendar`,
`randomizedDelaySec`, `extraArgs`.

## Development

```bash
nix develop
go vet ./...
go test -race ./...
```

## Security

- The refresh token is read from the environment or a systemd credential, never
  from the JSON config.
- Image downloads are restricted to an allowlist of known image hosts (pixiv
  `i.pximg.net`, konachan and danbooru CDNs); a source with a custom `api_base`
  relaxes this to any `https` host. Pagination cursors are restricted to the
  pixiv API host.
- Downloaded files are validated by `Content-Type` and magic bytes.

## License

[MIT](LICENSE). The Windows x64/x86 release archives bundle Tesseract OCR;
see [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md) for its components and
licenses.
