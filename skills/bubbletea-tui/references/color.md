# Colour: roles, ramps, and the four palettes you are really designing

## Roles, Never Hex

A screen asks for `t.S.Muted` or `t.P.Error`, never `"#8A93A3"`. The theme file owns
every colour (`kit/themes/*.json`), and `theme.Resolve` turns it into styles. If a screen
needs a colour the roles can't express, add a role to the contract. Do not reach for a
literal.

| Role | Job | Floor (WCAG vs its bg; `palette check`'s, see `palette -h`) |
|---|---|---|
| `primary` | the brand hue: focus bar, selection, current, header motif | 3.5 as text |
| `secondary` | second voice: primary button fill, gradient end | 3.0 |
| `tertiary` | a third voice: inline code, links; prompts and carets where primary is under 3:1 on `bgOverlay` (else they take the focus ink) | 4.5 |
| `accent` | section titles: reads "heading", never "alert" | 4.5 |
| `bg › bgPanel › bgSubtle › bgOverlay` | the background ramp | bgSubtle 1.1–2.2 |
| `fg › fgSoft › fgMuted › fgSubtle` | the text ramp | 7 › (unchecked) › 4.5 › 2.6; on bgSubtle fg 4.5, fgMuted 3.0 |
| `border` / `borderFocus` | rules: quiet, but seen / the one dialog's border | 1.3–1.9, a warning under 1.5 (light pages the same: 1.5:1 is ~15 L* on either) / unchecked |
| `success error warning` / `info` | state only | 3.5 / 3.0 |
| `onPrimary onWarning` / `onSecondary onError onSuccess` | text on a filled role | 4.5 / 3.5 |
| `fgSelected` | text on the primary selection fill | 3.5 (not under `focus: fg`) |

## Derive The Palette From The Brand, Then Check It

```
palette derive "#FF5A1F" "#3D7CC9" --bg "#0D1724" --light-bg "#F4F6F8" --swatch s.ansi
cellshot s.ansi s.png      # look at it
palette check theme.json   # on any existing theme
```

`derive` works in OKLCH so lightness steps are perceptual. It:

- keeps the brand's hue and chroma for `primary`/`secondary` and moves only lightness
  until the colour clears its floor; puts `border` at 1.5:1, where a rule is seen
  without being a line (`#2A231A` on `#17140F`, 1.18:1, passed the old floor with every
  rule invisible on the sheet)
- **tints every neutral toward the background's hue** (or the primary's, on a neutral
  bg). This is the single biggest difference between a palette that reads "designed" and
  one that reads "default grey".
- pulls status colours into one lightness and chroma band, so green, red and amber sit
  together instead of shouting at different volumes
- picks the on-colour ink (light or dark) by contrast for every fill, and on a mid-tone
  fill where neither clears its floor deepens that ink toward black or white along its
  hue (`theme.OnInk`, which the engine uses too), so the brand hex stays the fill
- from one brand hue makes `secondary` and `tertiary` lightness steps of it (and
  `accent` its pale tint), not a hue shift and a complement the brand never chose; with
  two hues the second is the brand's and the tertiary a complement
- writes both a dark and a light set, the nearest xterm-256 index per role, and the
  `ansi16` pins its own `check` would ask for (most brands' warning lands on error's red
  at 16 colours), so the first `palette check` of the theme prints no 16-colour pin.
  The brand takes the slot nearest its hue first (an amber on yellow 11/3) and a
  displaced warning moves; only a brand that is a status role's own colour (a yellow,
  within 25° of the slot's hue) yields it
- warns when `warning` sits beside `primary` in hue and chroma (an amber brand's warning
  a shade brighter than its amber): a warning row then reads as a focused one

Then tune by hand. The tool gets you a correct palette; the brand read decides whether
the tertiary should be a complement or a quiet tint, and what an amber brand's warning is.

**Measure, don't guess.** Take brand colours from the logo file, the site's CSS or the
style guide. A "close enough" blue is a different brand.

## Two Ramps, Small Steps

Crush's four background steps are ~5–8 L apart in OKLCH, and its border is the second
step. Small steps look calm. If you can see the panel edge from across the room, the step
is too big.

- `bgPanel` (+~0.035 L): recessed blocks (tool output, code, thinking, cards)
- `bgSubtle` (+~0.075 L): unselected buttons, inactive chips, a dim focus bar
- `bgOverlay` (+~0.13 L): popovers, completions, hover (`ui.Popover`: the command palette)

## Light Mode Is Not An Inversion

Keep the hue and shift the lightness. Primary usually gets darker and more saturated to
clear 3.5:1 on a near-white. Warnings move from yellow to a dark amber (yellow text on
white is invisible). Borders become *darker* than bg. Test the light set as carefully as
the dark one; most themes ship with an unreviewed light set.

Detect the terminal: `tea.RequestBackgroundColor` in `Init`, then on
`tea.BackgroundColorMsg` call `theme.ResolveProfile(file, msg.IsDark(), ascii, profile)`,
with the profile kept from `tea.ColorProfileMsg`. Never assume dark.

## Painting The Background Is A Brand Decision

`expressions.paintBg: true` sets `tea.View.BackgroundColor`, so the whole terminal takes
the brand's bg (Crush does this). It is a strong identity move, but it:

- ignores the user's terminal theme and transparency
- obliges you to review both role sets (light users get your bg too)
- needs every filled span to carry its own background, or resets punch holes. Draw on
  the canvas and `Backfill`, or use `ui.OnBg` for strings.

Most tools should not. A product with a strong visual brand (a game tool, an AI agent with
a mascot palette) may.

## Gradients

- `theme.Grad(text, a, b, bold)` blends per grapheme cluster in CIELAB
  (`lipgloss.Blend1D`).
- Use it on 5–40 cells: the wordmark, a motif rule, a spinner, a progress fill. Never on
  prose, borders, or backgrounds.
- Two stops from the brand, adjacent in hue. A three-hue rainbow is a toy.
- Under ASCII/NO_COLOR it degrades to plain bold text.

## Derived Tints Instead Of New Colours

Need a diff background, a selection tint, an alert bar? Blend a role toward bg
(`theme.Mix(p.Success, p.Bg, 0.82)`) instead of inventing a hex. Crush derives its diff
backgrounds this way from one hue (20%/13% for inserts, 15%/8% for deletes), which is why
any theme gets harmonious diffs.

## Foreign Colour

Subprocess output (`ls --color`, test runners, logs) arrives with the ANSI-16 palette of
someone else's terminal. Crush remaps the 16 basic colours onto on-brand legible ones
before drawing. At minimum, render foreign output on `bgPanel` so it reads as a quotation.

## The Four Palettes

TrueColor is what you design. ANSI256, ANSI16 and none are what many users get. Check:

```
go run ./cmd/<app> -out frames -profile ansi256
go run ./cmd/<app> -out frames -profile ansi
go run ./cmd/<app> -out frames -profile ascii
```

and look at the PNGs. What usually breaks:

- the `bg`/`bgSubtle` step merges at 256 colours, so selection disappears. Move it further
  apart, or add the glyph marker.
- a tinted neutral downsamples by distance onto a hue at 16 colours: a warm page lands on
  yellow, warm hints on red, and three cool text levels on one blue; a warm `borderFocus`
  one notch lighter than its neighbour draws a ruled dialog's edge in error's red. The
  kit puts every neutral on a grey there (`bg` 0/15, `fg` the opposite, `bgSubtle` and
  `border` beside the page, `borderFocus` a step up on `fgMuted`'s grey), the roles a
  dialog draws included. Dark keeps three text levels at 16 colours (`fgMuted` 7, `fgSubtle` 8);
  light keeps two (`fgMuted` and `fgSubtle` both 8), because ANSI 7 on a white page is
  below every floor; `fgSubtle` is never the only carrier of meaning. `palette check`
  fails a pin that puts a neutral back on red and warns on any other hue or two text
  levels pinned onto one grey.
- `border`, a step off `bg`, lands on `bg` itself at 16 colours and every rule vanishes.
  `theme.ResolveProfile` moves a rule or border colour that isn't `Distinct` from bg to
  ANSI 8 (dark) or 7 (light); keep the profile from `tea.ColorProfileMsg` and pass it, or
  the rules go. At 256 colours every kit theme's border stays apart from bg; for a theme
  whose border does land on bg's index there, the fallback steps it toward fg.
- at `ascii`, everything is the same colour. Focus, selection, error and disabled must
  still be visible through glyphs, bold, reverse video and words.
- two state roles land on one of the 16 colours: downsampling takes the nearest of 16,
  so an amber `#D99A3B` and an error `#EF8482` are both ANSI 9, and most yellows go red.
  No hex fixes it without leaving amber at 256 colours. Pin the role for 16 colours only:

  ```json
  "ansi16": { "dark": { "primary": 11, "warning": 3 }, "light": { "primary": 0 } }
  ```

  `palette check` names each clash and the pin that fixes it (stderr): it fails error
  off red, under `focus: fg` primary on red or on another state's colour, and a neutral
  (`bg`, `fg`, `fgMuted`, `fgSubtle`, their steps, `border`, `borderFocus`) pinned onto
  red; it warns
  on the rest, two of `fg`/`fgMuted`/`fgSubtle` pinned onto one grey included
  (`palette -h` lists them). A warning is not a pass to ignore: pin it unless
  the brand accepts it in its read. Pin bright (9-14) on dark, normal (1-6) on light,
  error to 9/1, a status role to its own slot (warning 3/11, success 2/10, info 4/12 or
  6/14), never a grey but for a warm brand on a light page (below); the user's terminal
  palette decides the exact colour. A pin that moves a role more than 60° of hue while
  a nearer slot is free (a rose brand on blue 12, with magenta 13 free) warns with the
  hex, the slot and the nearer pin: the brand reads as info there.
- a warm brand (OKLCH 25°–85°: coral, orange, copper, amber) has no slot of its own at
  16 colours: red is error's, yellow the warning's, magenta the pink an orange is not.
  On a dark page it takes yellow 11 and the warning keeps yellow 3, so every warning mark
  carries `▲`/`!`; on a light page it has no hue (`primary` pinned to 0, fg's grey: yellow
  3 is 1.35:1 on white) and the kit draws focus as fg and bold. Its secondary, tertiary
  and accent follow it off red and magenta. The warning never leaves yellow: `palette
  check` fails it on green, and a pinned brand under 3:1 as text or 4.5:1 under its
  selection ink (white on yellow 11 is 1.17:1), measured in the slot's colour.

## DO / DON'T

- **DO** derive every role from 1–3 measured brand colours. **DON'T** start from Lip
  Gloss's example purple or Bubbles' default pink.
- **DO** tint neutrals toward the brand. **DON'T** use `#888`.
- **DO** pin a state role with `ansi16` when `palette check` says two states share one
  of the 16 colours. **DON'T** move the brand's hex to dodge downsampling: it changes
  every other profile to fix one.
- **DO** keep hue for state and brand, and lightness for hierarchy. **DON'T** colour
  columns to tell them apart.
- **DO** check contrast for every text role on every background it sits on (fg on
  bgSubtle too). **DON'T** trust a palette you haven't run through `palette check`.
- **DO** give every filled element an explicit on-colour. **DON'T** put white on yellow.
