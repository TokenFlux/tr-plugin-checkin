# TokenRouter Daily Check-in

A TokenRouter plugin that gives users a daily balance reward and lets administrators
inspect check-in history. Each claim uses host KV and a stable wallet idempotency key,
so retries retain the original reward amount and completed claims cannot pay twice.

## Install

Choose `tokenflux.checkin` in the TokenRouter plugin catalog, review `wallet.credit`,
and install a compatible version. The example requires plugin API v1 and a host
version of at least 0.6.0. Configure the reward and administrator wallet limits before
using it with real accounts.

The default reward is 0.1 internal balance units. The host supplies its timezone,
currency display, authenticated user, and optional notice. A zero reward records a
check-in without issuing a wallet adjustment. Ordinary uninstall can retain business
records for later installation; code rollback does not roll back wallet transactions.

## Build and develop

Use the versions declared in `go.mod`, `.node-version`, and `web/package.json`.
Run `make bootstrap` once after cloning if you need to regenerate dependency locks,
then `make test pack`. The release workflow builds Linux amd64/arm64, macOS amd64/arm64,
and Windows amd64, validates every archive, and publishes checksums with the manifest.

`make dev` builds a local package directory for an isolated host's `plugins.dev_dirs`.
Enable `plugins.dev_mode` only in development. Browser-only development uses the SDK
mock; its balances and records do not establish real host persistence.

This plugin is MIT licensed. It is administrator-trusted native and browser code;
process isolation does not provide an operating-system sandbox. See the
[plugin development guide](https://github.com/TokenFlux/TokenRouter/tree/main/docs/guides/plugins)
for the SDK and distribution contracts.

The calendar artwork comes from [Lucide](https://github.com/lucide-icons/lucide/blob/66d8f9fc394b8530377e5f6112f0b8908ba01280/icons/calendar.svg).
Its license and attribution are retained below and in `assets/LICENSE.lucide`.

```text
ISC License

Copyright (c) 2026 Lucide Icons and Contributors

Permission to use, copy, modify, and/or distribute this software for any
purpose with or without fee is hereby granted, provided that the above
copyright notice and this permission notice appear in all copies.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

---

The following Lucide icons are derived from the Feather project:

airplay, alert-circle, alert-octagon, alert-triangle, aperture, arrow-down-circle, arrow-down-left, arrow-down-right, arrow-down, arrow-left-circle, arrow-left, arrow-right-circle, arrow-right, arrow-up-circle, arrow-up-left, arrow-up-right, arrow-up, at-sign, calendar, cast, check, chevron-down, chevron-left, chevron-right, chevron-up, chevrons-down, chevrons-left, chevrons-right, chevrons-up, circle, clipboard, clock, code, columns, command, compass, corner-down-left, corner-down-right, corner-left-down, corner-left-up, corner-right-down, corner-right-up, corner-up-left, corner-up-right, crosshair, database, divide-circle, divide-square, dollar-sign, download, external-link, feather, frown, hash, headphones, help-circle, info, italic, key, layout, life-buoy, link-2, link, loader, lock, log-in, log-out, maximize, meh, minimize, minimize-2, minus-circle, minus-square, minus, monitor, moon, more-horizontal, more-vertical, move, music, navigation-2, navigation, octagon, pause-circle, percent, plus-circle, plus-square, plus, power, radio, rss, search, server, share, shopping-bag, sidebar, smartphone, smile, square, table-2, tablet, target, terminal, trash-2, trash, triangle, tv, type, upload, x-circle, x-octagon, x-square, x, zoom-in, zoom-out

The MIT License (MIT) (for the icons listed above)

Copyright (c) 2013-present Cole Bemis

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

```
