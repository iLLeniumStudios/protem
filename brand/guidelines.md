# Protem — Logo Guidelines (compact)

## 1. The logo
- **Idea**: two brackets close a square around one open gap and one nearly-shut gap — access granted for a while, closing on its own. No letterform, no initials: the mark stands for the idea, not the name.
- **Versions**: horizontal lockup (symbol + wordmark) · stacked lockup · symbol-only (app icon / favicon) · wordmark-only
- **Files**: `logo/*.svg` are the masters (screen and print both use these — the mark is flat colour, no gradients, so there is no separate CMYK file). Need print? Open the SVG in any vector editor and export PDF/EPS directly from `protem-symbol-black.svg` / `protem-horizontal-black.svg`.

## 2. Clear space
Keep a clear zone of **1 × bracket stroke width** (the width of either bracket arm — 40 of the symbol's 256 units, i.e. ~15.6 % of the symbol's own width) around the logo on all sides. The zone scales with the logo: double the size, double the clear space.

## 3. Minimum size
| Version | Screen |
|---|---|
| Horizontal lockup | 120 px wide (below that, drop to symbol-only) |
| Symbol (regular master, `protem-symbol*.svg`) | 24 px |
| Symbol (small-size cut, `protem-symbol-small.svg` / favicon files) | 16 px |

Below 24 px always use the small-size cut, not the regular master scaled down — its wider gaps keep both openings legible where the regular master's narrower gap closes into a grey seam.

## 4. Colour

| Name | HEX | RGB | Use |
|---|---|---|---|
| Protem Blue (primary, light surfaces) | `#1D5FA8` | 29, 95, 168 | Symbol on white or light backgrounds; the UI's buttons and links |
| Protem Blue Light (primary, dark surfaces) | `#4888D5` | 72, 136, 213 | Symbol and links on dark backgrounds |
| Ink (neutral) | `#14181C` | 20, 24, 28 | The wordmark on light backgrounds; body text |

Contrast is computed with the WCAG relative-luminance formula:
- Protem Blue `#1D5FA8` on white `#FFFFFF`: **6.45 : 1** (and white on Protem Blue the same).
- Protem Blue Light `#4888D5` on the dark background `#0d1013`: **5.23 : 1**; on the dark surface `#15171b`: **4.92 : 1**.
- Ink `#14181C` on white: **17.84 : 1**.

These are the UI's accent colours (`web/src/styles/tokens.css`), so the logo and the product match without a separate palette. Blue is common in this category; the owner chose it over amber (which is the UI's warning colour) and violet (close to Teleport's purple). The mark carries the identity, not the hue, and the colour can change later without redrawing anything.

**Approved pairs**: blue symbol with Ink wordmark on white or light grey (`protem-horizontal.svg`) · light-blue symbol with white wordmark on dark (`protem-horizontal-on-dark.svg`) · all white on Protem Blue or on any dark surface (`-white.svg`) · all black on white (`-black.svg`) · all blue on white (`-mono-1d5fa8.svg`). For a symbol alone on a dark background use `protem-symbol-reversed-master.svg`, drawn slightly thinner because white shapes look heavier on dark.

## 5. Typography
- The wordmark "Protem" is set in **Public Sans Bold** and converted to outlines, with the capitals 56 % of the symbol's height and centred on it. Public Sans is the typeface the UI already uses; it is licensed under the SIL Open Font License 1.1, which permits this use. The logo files contain paths only, so no font needs to be installed.
- Surrounding UI and body text: Public Sans, with IBM Plex Mono for code.

## 6. Don'ts
Don't stretch or squash the lockup · don't recolour outside Protem Blue, Protem Blue Light, Ink, black and white · don't rotate the symbol (turned 180° the open gap and the closing gap swap, which reverses the idea) · don't add shadows, outlines, gradients or bevels · don't change the gap between symbol and wordmark, or resize one without the other · don't place the logo on a busy or mid-tone photograph without a solid tile behind it · don't retype "Protem" in another font for logo use; use the outline files.

## 7. What has not been checked
- **Trademark clearance.** "Protem" and the mark have not been checked against trademark registers. Run a professional clearance search before relying on the name or mark.

## 8. Contact
Master files: `brand/logo/` (see `brand/README.md` for the full file list).
