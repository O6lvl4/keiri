# keiri

Bookkeeping document hygiene CLI.

`keiri` keeps receipts, invoices, and other bookkeeping documents tidy:
canonical filenames, lint for naming-rule violations, and (eventually)
reconciliation against credit-card statements.

Today it ships **receipts**. Invoices, contracts, and payroll slips are
on the roadmap.

## Install

Requires Go 1.22+.

```sh
go install github.com/O6lvl4/keiri/cmd/keiri@latest
```

Or build from source:

```sh
git clone https://github.com/O6lvl4/keiri
cd keiri
go build -o keiri ./cmd/keiri
```

## Use

### Ingest

Auto-classify a downloaded PDF: extract its text via `pdftotext`, find
the first matching rule under `.keiri.yaml`'s `ingest.rules`, and
move the file to its canonical destination with a templated name.

```sh
keiri ingest ~/Downloads/2026-04-19.pdf            # one file
keiri ingest --dry-run ~/Downloads/*.pdf           # preview many
```

Requires `pdftotext` (install via `brew install poppler`).

Filename templates support these placeholders:

| placeholder    | example value          |
| -------------- | ---------------------- |
| `{yyyy-mm-dd}` | `2026-04-19`           |
| `{yyyymmdd}`   | `20260419`             |
| `{yyyy-mm}`    | `2026-04`              |
| `{yyyymm}`     | `202604`               |
| `{yyyy}`       | `2026`                 |
| `{original}`   | input filename (no ext)|
| `{ext}`        | input extension        |

Date is taken from the original filename if it contains
`YYYY-MM-DD` / `YYYYMMDD` / `YYYY年M月D日`, else from the PDF text,
else from mtime.

```yaml
# .keiri.yaml
ingest:
  rules:
    - match: "American Express"
      dest: "クレジットカード"
      name: "amex-{yyyy-mm-dd}.pdf"
    - match: "Anthropic"
      dest: "契約サービス関連書類 - 月額/Anthropic"
      name: "Anthropic_{yyyymmdd}_receipt.pdf"
    - match: "Google Workspace"
      dest: "契約サービス関連書類 - 月額/GoogleWorkspace"
      name: "GoogleWorkspace_{yyyymmdd}_invoice.pdf"
```

Existing destination files are skipped (no overwrite). Ingest places
files at the configured `dest`; moving them into a subfolder like
`done/` (to mark "this one's reconciled") is left to the human.

### Catch-up

Print exactly what's missing for required categories and where to
fetch each one. Optional flag launches the unique portal URLs in the
default browser.

```sh
keiri catchup            # list missing months + portal URLs
keiri catchup --open     # also open each portal in the browser
```

Portal URLs are declared in `.keiri.yaml`. Each entry can be either a
bare URL string, or an object with an optional Chrome profile so each
portal opens with the right Google account:

```yaml
portals:
  クレジットカード:
    url: https://global.americanexpress.com/dashboard
    chrome-profile: alice@example.com
  契約サービス関連書類 - 月額/GoogleWorkspace:
    url: https://admin.google.com/ac/billing/manage
    chrome-profile: alice@example.com
  # bare URL form still works:
  契約サービス関連書類 - 月額/OpenAI: https://platform.openai.com/account/billing/history
```

`chrome-profile` accepts:

- an **email address** — keiri reads
  `~/Library/Application Support/Google/Chrome/Local State` (and falls
  back to per-profile `Preferences`) to find the matching directory
- a **literal profile directory name** (`Default`, `Profile 24`, …)

Override the Chrome binary path via `KEIRI_CHROME_BIN` if needed.

Sample output:

```
⚠ Catch-up needed (8 required categories)

  クレジットカード
    missing: 202604
    portal:  https://global.americanexpress.com/dashboard
  契約サービス関連書類 - 月額/GoogleWorkspace
    missing: 202602 202603 202604
    portal:  https://admin.google.com/ac/billing/manage
  ...
```

### View

Render the inventory as a self-contained HTML report and open it.

```sh
keiri view              # writes ~/.cache/keiri/view.html, opens it
keiri view --out r.html # write somewhere else
keiri view --no-open    # don't launch a browser
keiri view --out -      # stream HTML to stdout
```

The default path is stable, so once you've opened
`~/.cache/keiri/view.html` you can bookmark it and re-run `keiri view`
to refresh the same tab.

Required rows are highlighted, missing months in required categories
are red, multiple-file months are blue, complete cells are green.
Dark mode follows the OS preference.

### Inventory

Walk a bookkeeping root, print a category × month coverage matrix,
and warn about gaps in recurring categories.

```sh
keiri inventory                       # default root: ~/Downloads/経理-gdrive
keiri inventory --depth 2             # expand to vendor level (e.g. 月額/GoogleWorkspace)
keiri inventory --dir <path>          # custom root
keiri inventory --months 12           # show last 12 months (0 = all)
keiri inventory --gap-threshold 0.9   # stricter "recurring" cutoff
keiri inventory --no-matrix           # gaps only
```

`·` in a cell means "no document for that month/category". With a
config file (below) `★` marks **required** categories and `·` marks
**optional** ones in the matrix.

#### Config file

Drop a `.keiri.yaml` at the bookkeeping root to declare which
categories are required vs optional. Required categories always show
up in the gaps report regardless of how recurring they look; optional
ones never do.

```yaml
# ~/Downloads/経理-gdrive/.keiri.yaml
inventory:
  depth: 2
  required:
    - クレジットカード
    - 売上請求書
    - 請求
    - 契約サービス関連書類 - 月額/Anthropic
    - 契約サービス関連書類 - 月額/GoogleWorkspace
    - 契約サービス関連書類 - 月額/OpenAI
  optional:
    - 個人管理系
    - 契約サービス関連書類 - 月額/Apple
    - 契約サービス関連書類 - 月額/xAI
    - 旅費・交通費関連
```

Pattern matching is path-prefix based — `個人管理系` matches everything
underneath. Without the config file, the gaps report falls back to
`--gap-threshold` coverage detection.

#### Output example

```
⚠ Gaps (required categories only)
  クレジットカード (complete)
  売上請求書 (complete)
  契約サービス関連書類 - 月額/Anthropic (complete)
  契約サービス関連書類 - 月額/GoogleWorkspace missing 2 month(s): 202602 202603
  契約サービス関連書類 - 月額/OpenAI missing 1 month(s): 202603
  請求/Acme_Inc (complete)
```

Date extraction recognizes `YYYYMMDD`, `YYYYMM`, `YYYY-MM-DD`,
`YYYY-MM`, and `YYYY年M月` forms; year is constrained to `20XX` so
4-digit identifiers don't get misread. Subdirectories named `done`
are flattened into their parent so `クレジットカード/done/...` shows
as just `クレジットカード`.

### Receipts

```sh
keiri receipts lint     # report naming-rule violations
keiri receipts plan     # show proposed renames (no writes)
keiri receipts apply    # execute the renames
```

Default directory is `~/Downloads/経理/領収書`. Override with `--dir` or
`$KEIRI_RECEIPTS_DIR`.

The vendor-normalization map is embedded
([`internal/receipts/vendors.tsv`](internal/receipts/vendors.tsv)).
Lint detects:

- missing `YYYYMMDD_` date prefix
- redundant double dates (period strings and order IDs are excluded)
- copy traces (`のコピー`, `[N]`, ` (N)`)
- decoration / wobble characters (`｜ － – `, padded `_`)
- extension distribution
- monthly receipt counts

`plan` produces canonical filenames following
`YYYYMMDD_<Vendor>_<identifier>.<ext>`. Heuristics:

- vendor names are normalized via the embedded map (`airdo→AIRDO`,
  `SNA→ソラシドエア`, `Paddle.com→Paddle`, …)
- `(N)` becomes `_vN`; `のコピー` and `[N]` are dropped
- noise words (`領収書`, `Your`, `Order`, `receipt`/`Receipt` as a
  standalone token) are stripped — but `Receipt-XXXX-XXXX` identifiers
  are preserved
- when a date prefix is missing the file's mtime is used

## Roadmap

- [x] receipts: lint / plan / apply
- [x] inventory: monthly coverage matrix
- [x] inventory: gap detection for recurring categories
- [x] inventory: vendor-level depth and `.keiri.yaml` required/optional
- [x] inventory: surface the current expected month
- [x] view: HTML report for at-a-glance coverage
- [x] view: clickable missing cells / gap rows
- [x] catchup: list missing docs + open billing portals
- [x] ingest: auto-classify PDFs via pdftotext + rules
- [ ] receipts: PDF metadata extraction (vendor / amount / date) via `pdftotext`
- [ ] receipts: configurable vendor map (load external `vendors.tsv`)
- [ ] reconcile: match credit-card statements (CSV / PDF) against receipts
- [ ] invoices: lint / plan / apply (issued and received)
- [ ] contracts: subscription-document organization
- [ ] payroll: monthly archive helper
- [ ] Homebrew tap

## Companion tools

- [`grclone`](https://github.com/O6lvl4/grclone) — git-style CLI for
  rclone. Pair with keiri when your bookkeeping documents live in a
  cloud-synced folder.

## License

MIT
