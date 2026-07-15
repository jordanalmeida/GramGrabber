# Design

Visual system of the GramGrabber landing page (`docs/`). Theme: **"night flight"** — a paper plane crossing a deep cobalt sky; instrument-panel calm, product-grade polish (not neon-hacker).

## Color (OKLCH)

| Token | Value | Use |
|---|---|---|
| `--bg` | `oklch(11.5% 0.03 262)` | body background (cobalt-tinted night) |
| `--bg-deep` | `oklch(8% 0.025 262)` | nav, footer, hero top |
| `--surface` | `oklch(16% 0.035 262)` | cards, panels |
| `--surface-2` | `oklch(20.5% 0.042 262)` | step number chips |
| `--line` | `oklch(29% 0.045 262)` | borders |
| `--ink` | `oklch(95% 0.012 262)` | body text (≥7:1 on bg) |
| `--muted` | `oklch(76% 0.035 262)` | secondary text (≥4.5:1) |
| `--primary` | `oklch(55% 0.2 262)` | cobalt — CTA fills (white text) |
| `--primary-bright` | `oklch(73% 0.145 262)` | links on dark |
| `--accent` | `oklch(83% 0.115 200)` | cyan — "instrument glow": highlights, plane, carets |
| `--success` | `oklch(80% 0.16 152)` | terminal ✓ / speeds |

Strategy: **Committed** — dark cobalt surface carries the brand; cyan accent strikes the analogous "cockpit display" relationship.

## Typography

- **Archivo** (variable, `wdth` axis): everything. Headings at `wdth 118`, weight 750–800, letter-spacing −0.02…−0.028em. Body 1.0625rem/1.7.
- **Spline Sans Mono**: terminal demo, code blocks, data visuals only (functional mono, not costume).
- Loaded from Google Fonts with `display=swap` + preconnect.

## Signature elements

- Hero: canvas night sky, 3 star layers with scroll parallax + occasional data-streak meteors; animated truthful terminal demo (typing → channel picker → 8-thread progress bars).
- Section divider: dashed SVG flight path; paper plane rides the curve on scroll.
- Features as alternating rows with live-looking mono visuals (thread bars, resume demo, channel picker, MTProto diagram) — no icon-card grids.
- Tutorial: numbered steps (real sequence) with dashed connector line and copy-to-clipboard code blocks.
- All motion honors `prefers-reduced-motion` (static sky frame, final terminal frame, no reveals).

## Layout

- Container: `min(1120px, 100% − 3rem)`.
- Reveals are JS-enhancement only (`.js .reveal`), content visible without JS; 2.5s safety timeout.
- Known trap fixed twice: grid items default `min-width:auto` — wide `<pre>`/terminal content must sit in `min-width:0` items or block flow, with `overflow-x:auto` on the `pre`.
