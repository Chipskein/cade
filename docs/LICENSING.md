# Licensing

**English** · [Português](LICENSING.pt-BR.md)

Review of cade's license and of what goes into the `cade` binary, done for [#10](https://github.com/Chipskein/cade/issues/10) on 2026-09-28. This is an engineering review, not legal advice.

## Decision

cade moves from the **GPLv2** to the **GPLv3 or later** (SPDX `GPL-3.0-or-later`), starting with v0.1.0. v0.0.0 stays under the GPLv2 for whoever received it.

- **Nothing forces the move:** every component in the binary is under a permissive license and works with either version.
- **The GPLv2 is closing doors the roadmap needs:** it is incompatible with Apache-2.0 and with (A)GPLv3 code. The PDF search in the roadmap has to pick an extraction and rendering library, and good candidates are Apache-2.0 (pdfium) or AGPL-3.0 (MuPDF). Many Go libraries are Apache-2.0 as well.
- **The move is possible now:** every commit comes from the project's owner (including the Copilot agent's commits in the owner's own PRs). There are no outside contributors to ask; with them, the change would need each one's consent.
- **"or later"** follows the FSF's recommended notice. The old `LICENSE` held the GPLv2 text with no version notice, which left open whether it was "only" or "or later"; the SPDX identifier now removes that doubt.

## Current state before the change

- `LICENSE` had the GPLv2 text (June 1991), no notice in the sources, and `THIRD_PARTY_NOTICES.md` and the CHANGELOG said "GPLv2".
- The release archive (`go tool mage dist`) ships `LICENSE` and `THIRD_PARTY_NOTICES.md` next to the binary.

## What is in the binary

Taken from `go version -m bin/cade`, from `go.mod` and from the cgo `LDFLAGS` of `internal/llm/llamacpp`.

| Component | License | GPLv2 | GPLv3 |
|---|---|---|---|
| Go standard library and runtime | BSD-3-Clause | yes | yes |
| llama.cpp and ggml (`b11195`) | MIT | yes | yes |
| Libraries vendored by llama.cpp in `mtmd` and `vendor-hash` (stb_image, miniaudio, xxHash, rotate-bits, sha1, sha256, sheredom/subprocess) | MIT, MIT-0, BSD-2-Clause, public domain, Unlicense | yes | yes |
| mattn/go-sqlite3 v1.14.52 | MIT | yes | yes |
| SQLite (bundled with go-sqlite3) | public domain | yes | yes |
| sqlite-vec via sqlite-vec-go-bindings v0.1.6 | MIT or Apache-2.0, used under MIT | yes, under MIT only | yes, under either |
| klauspost/compress v1.20.1 (`snappy`, `s2`, `gzip`, `flate`, `zstd`) | BSD-3-Clause | yes | yes |
| andybalholm/brotli v1.2.6 | MIT | yes | yes |
| golang.org/x/image v0.46.0 (`webp`, `draw`) | BSD-3-Clause | yes | yes |

The vendored llama.cpp libraries were not listed in `THIRD_PARTY_NOTICES.md` before this review; they are now, with the texts that require a notice.

## Outside the binary

| Item | License | Why it does not affect cade's license |
|---|---|---|
| Mage v1.17.2 | Apache-2.0 | a `tool` dependency: it builds cade and is never linked into it (`internal/buildinfo` already skips it) |
| Models (nomic-embed-text-v2-moe, Qwen3.5-2B and its `mmproj`) | Apache-2.0 | data downloaded apart by `go tool mage models`, never shipped with cade |
| Qwen2.5-3B-Instruct (old default) | Qwen Research License, non-commercial | no longer downloaded; a config pointing at it runs under that license |
| NVIDIA CUDA runtime and cuBLAS | proprietary | only the local CUDA build links them; see the exceptions |

## Restrictions and exceptions

- **CUDA builds are not distributed.** Releases are CPU only. Neither the GPLv2 nor the GPLv3 clearly treats the CUDA libraries as system libraries, so shipping a CUDA binary would need an explicit linking exception from the copyright holder first. Building it locally is fine: the GPL only sets conditions on distribution.
- **GPL-2.0-only code can no longer come in.** It cannot be combined with the GPLv3. "GPL-2.0-or-later" code still can.
- **sqlite-vec** stays under MIT; its Apache-2.0 option is now also available, but changes nothing.
- **Models are separate works** and keep their own licenses; a new default model must be checked on its card and in the GGUF's `general.license`, as in phase 18.

## Checking a new dependency

A module or library linked into cade must be compatible with `GPL-3.0-or-later`:

- **Yes:** MIT, MIT-0, BSD, ISC, zlib, Apache-2.0, MPL-2.0, public domain / Unlicense / CC0, LGPL (2.1 or later, 3), GPL-2.0-or-later, GPL-3.0. AGPL-3.0 is allowed too (GPLv3 §13), but that part keeps its network clause.
- **No:** GPL-2.0-only, EPL-1.0, CDDL, SSPL, proprietary, non-commercial or "Commons Clause" terms.

Then add it to `THIRD_PARTY_NOTICES.md` with its license text; `internal/buildinfo` fails when `go.mod` gains a module the file does not name.
