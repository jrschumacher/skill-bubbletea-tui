# Components: anatomy and rules

Every component here exists in `kit/ui`. Copy it and change what the task needs. Each
entry gives the anatomy, the rules that a cold rebuild got wrong, and the kit function.

## Header — `ui.Header(t, w, h, brand, details[, right])`

One row (or a banner, if the brand earns it and h ≥ 30): the wordmark, the brand motif
filling the gap, and details on the right (`env · region · selection`, the first part
in fg and the rest muted). It is drawn per `expressions.header`: `rule`, `bar` (a filled
strip), `minimal` (`brand · details`, and `right` at the edge: a count, a status),
`stamp` (the name as an inverse stamp at the right), or `banner`. Details truncate
before the brand does. The one-row header is the kit's default; a brand read's move may
replace it (`brand.md` § Move A Region).

## Tabs — `ui.Tabs`

Text, never boxes, the names in the theme's case (`labelCase` when set, else `titleCase`).
The active tab is bold with a marker drawn per `expressions.tabs`
(a `▔` underline row, a pill, `[brackets]`, or a ` / ` path). Counts sit beside the
names in subtle; a count is only red if it needs attention now.

## Section — `ui.Section(t, title, w, info)`

`Title ───────── info`. The title is in the theme's title style and case, the rule is in
the border colour, and right info is subtle. This is how regions separate.

## List — `ui.List(t, items, sel, offset, w, h, focused)`

- The marker column is always 2 cells, so selecting never shifts text.
- Selection follows `expressions.selection`: `bar` (▌ + bold), `fill` (a full-width
  primary bar; every span carries the bg), `caret` (› + primary), `inverse`.
- Blurred lists keep the selection visible in a quieter style and its own shape:
  `t.BlurBar()` (`glyphs.blurBar`, else `╎`, ASCII `:`) in fgSubtle, never the focus bar
  or cursor in grey, which under NO_COLOR is a second focus. `CheckGlyphs` refuses a
  `blurBar` that is the `focusBar`.
- Title in fg, description muted after it (dropped first when tight), right-aligned
  hint in subtle, fuzzy matches underlined.
- Before the title, a status column (`Item.Mark`, a rung of the theme's ladder by its
  meaning, else `Item.State`'s Dot) and a muted `Item.Lead` column (a repo#n, an id) as
  wide as the widest lead, so titles line up. Don't hand-write a row for either.
- The scrollbar column appears only when the list overflows.
- Empty list: an empty state, never a blank area.

## Table — `ui.Table`

Borderless. The header row in subtle UPPERCASE (or the theme's case), one rule under it,
2-cell column gaps, numbers right-aligned, the first column taking the rest. The
selected row is marked like a list row. Color a cell only when its value is abnormal (a
red error rate), never a whole column.

## Detail Pane

A `Section` with the item name and its environment as info, a KV block (`ui.KV`, keys
muted and values in fg), then sections for the next most useful things (a sparkline with
its last value and trend, recent events). It appears at ≥ 110 columns. Below that it
collapses to one summary line under the list.

A body viewer in it (JSON, code, a response, a log) levels its text at two lightnesses:
names and keys in fg, punctuation and values in muted (or the reverse, chosen once),
never one flat level and never a rainbow of token colours. Two levels let the eye find
the keys; hue stays for state (color.md).

## Dialog / Command Palette — `ui.Dialog` + `Canvas.Overlay`

- The title row per `expressions.dialogTitle` (the motif rule in a gradient, plain, or a
  filled bar), then a blank row, the body, buttons, and one help line.
- The edge per `expressions.dialog`: a border (rounded, square, double, heavy), a filled
  panel (`none`, which gains a rule above and below where 16 colours or NO_COLOR drop the
  fill), or `rules`, the editorial one: a filled panel between two hairline rules
  (`── Send reply ──────`), a one-cell margin inside, no sides, the title in the top rule.
  Its edge is glyphs, so it holds in every profile.
- `ui.Dialog(t, w, h, …)` takes the rows it may use (the showcase passes those between
  the tabs and a row of air above the footer) and scrolls its body when it doesn't fit
  (`ui.Clip`: `↑ n more` above, `… n more` below; `DialogScrolled` for a screen with an
  offset, `DialogBodyRows` and `ClipMax` to clamp it), so no dialog reaches the footer.
- A dialog's status (the sending dots, `3 of 12`) is set right-aligned into its bottom
  edge, as a rules title is into the top one, so it takes no row and never crowds the
  buttons (`ui.DialogStatus`).
- Palette: a prompt input with a caret, then a fixed-height list (it doesn't jump while
  typing), with fuzzy matches underlined and shortcuts right-aligned.
- Only the front dialog takes input. `esc` closes it. The scrim dims the rest. Draw the
  page under it unfocused (`focused=false` to its lists, tables and fields, a subtle
  prompt and no caret in its input): one focus per frame, and a list keeps its
  selection under the blurred mark.
  `Canvas.Overlay` (and `Popover`) clears what would still read as live beside it: a
  one-cell halo, a word the dialog cuts, whole, with an ellipsis where its phrase was
  cut (`▲ notifications-…`, not `igh` or `1s` at the edge), a page rule on the dialog's
  edge rows, and, where no scrim is drawn (16 colours, NO_COLOR, ASCII, `scrim: none`),
  the rows the dialog spans.
- Max ~64–70 cols wide; fullscreen below the minimum size.

## Confirm — destructive actions

```
Roll back checkout-api? ───────────────
prod returns to the last healthy version.

now     v2.4.0  failing 12 min
after   v2.3.9  healthy for 3 days
impact  3 pods restart, one at a time (~90s)

                         Roll back   [ Cancel ]
```

The title is the question, naming the object. The body says what changes, with before
and after. The danger button is coloured even when unfocused. **Cancel is focused by
default.** `y`/`n` shortcuts for experts. Prefer undo to a confirm when the action is
reversible.

## Form Field

```
▌ Version                                   ← label: bold when focused, muted when not
▌ tag or commit; latest is v2.4.1           ← hint in subtle, always visible
▌ > v2.4.1▏                                 ← input: prompt in the focus ink, caret
```

The gutter bar (`ui.Gutter`) marks the focused field and spans all its rows. Radios
`◉ ○`, checks `■ □` (glyphs from the theme), selects as a `›` cursor list with a
description beside each option. Errors appear under the field in error colour, with
words. The primary button is filled because Enter triggers it, and the footer says so.

## Forms And Wizards — `ui.NewForm`, `ui.Confirm`, `ui.Steps`, `ui.Summary`

Huh does the input; the kit does the look and the frame around it. The reference is the
showcase's first-run setup (`go run ./cmd/showcase -first-run`, harness states `setup-*`).
Huh field titles are the app's text: the kit re-cases only its own fixed copy (`t.Copy`
for labels such as tab names in `labelCase`, `t.Sentence` for a sentence such as "Canceled."
in `titleCase`), so write a field's title in the brand's case.

```
H A L Y A R D                                         setup / 2/6 Sign-in
◆ Cluster ── ▲ Sign-in ── △ Alerts ── △ Review ── △ Connect ── △ Finish
Sign-in ──────────────────── staging-eu-west-1   Summary ──────── DRAFT
▌ Sign in with                                   Cluster
▌ ▸ Browser        SSO in your browser            staging-eu-west-1
▌   Device code    approve on another device      Sign-in        default
▌   Access token   paste one from the console     Browser
```

- **The frame is yours:** header bar with `Steps.Count()` ("2/6 Sign-in"), the step row
  (`Steps.Row`, done/current/todo glyphs from the theme), the form on the left at its
  measure, `ui.Summary` on the right (label fgMuted over value fg; untouched defaults in
  fgSubtle marked `default`; a `DRAFT`/`SAVED` tag). Below ~100 columns the summary becomes
  `ui.SummaryStrip`, one line.
- **Every step counts.** Connecting and finishing are steps, so the counter never repeats
  the review's number on three screens.
- **Review defaults to the forward action** (`ui.NewActions(0, "Continue", "Edit",
  "Cancel")`); `ui.Confirm(t, title, action, negative, def, &v)` for yes/no, action first.
- **Waits say what to do if nothing happens**: the URL to open by hand, the code, elapsed
  time, `esc cancel`; a timeout becomes a failure state with Retry.
- **Esc goes back** from step 2 on; on step 1 it asks before dropping typed input.

## Buttons — `ui.Button`, `ui.Buttons`

Per `expressions.button`: `pill` (padded filled blocks), `bracket` (`[ Save ]`, the
focused one with a cursor), `underline` (text), `inverse`. Mnemonic letters are
underlined. Buttons sit in a row, aligned per the layout (right in dialogs, left in
forms).

**One order, everywhere: the action first, the negative last** (`Roll back  Cancel`,
`Yes  No`, `Continue  Back`). It matches Huh's confirm and Crush, and a user who learns it
on one screen can press it blind on the next. A two-option select is not a confirm:
use the confirm, so the order can't flip. The focused default is the safe choice when the
action is destructive or irreversible, and the forward choice otherwise (Continue on a
review step, not Edit).

## Tags And Alerts — `ui.Tag`, `ui.Alert`

`Tag(kind, "FAILING")` is a chip. `Alert(kind, label, msg, w)` is a chip plus a tinted bar
across the width: the banner for "something is wrong with what you're looking at". One
alert per screen. If there are many, it's a list.

## Toast — `ui.Toast`

Paints over the footer row for ~4 s (a tag and a message on a tinted bar), then the help
returns. Never a new row, never a modal.

## Footer / Help — `ui.Footer`, `ui.Help`

An optional left badge (mode, `PROD`, `PLAN`), the help packed from `key.Binding`s by
importance, and right status text (`synced 4s ago`). Per `expressions.footer`: bare,
on a filled strip, or under a rule. Full help (`?`) expands into columns, grouped. The
badge's width and a space come out of the keys' 76 columns at 80: name it first in the
state's keys cell (`[plan] · **enter send** · …`) so `tuilint spec` [S-FOOTER] counts it.

## Stream (chat, logs, events) — `ui.Transcript`, `ui.UserMessage`, `ui.AssistantMessage`, `ui.ToolCall`, `ui.Thinking`, `ui.TurnFooter`

- The user's voice carries a bar: the focus bar while the turn is focused, the turn bar
  in muted when it isn't (a bold or coloured bar means focus, and a sent turn has none);
  the assistant's text is indented 2 and wrapped to the measure.
- Actions use the tool-call grammar (`idiom.md` §6). Collapsed output gets an honest
  "N lines hidden" affordance.
- Follow mode pins to the bottom. When scrolled up, show "↑ N more" at the top and
  a "new below" hint. The cut falls on a block boundary: where it would split a block, the
  marker row carries that block's head with the count right-aligned
  (`✓ Edit config/timeouts.go        ↑ 25 more  pgup`), so no output shows with its head
  off-screen (`ui.Transcript`).
- A turn ends with a section-rule footer (model, provider, duration): the kit's
  default; a brand read's move may replace it.

## Editor / Prompt

A rule above (the kit's default; a brand read's move may replace it), a prompt glyph
in the focus ink (`t.S.Prompt`: primary, or bold fg under `focus: fg`; tertiary where
primary is under 3:1 on `bgOverlay`; subtle when not focused), since where you type is
the focus,
continuation lines with a subtle `:`, and the real cursor via `View.Cursor`. Grow from 1
to ~8 rows with content, then scroll. The placeholder says what to type, in the brand's
voice.

## Status Column — `ui.Dot`, `ui.Busy`

One cell per row, before the name. `ui.Dot(t, state)` draws each state in its own shape
and role colour: ok `done`, current `current` (the open or loaded one: ● in the focus
ink), busy `partial`, warning `warn`, error `fail`, idle `todo`, so the shape says it
under NO_COLOR too, and `●` (`current`) keeps one meaning: now. A
state of the product's own (`new`, `reviewed`) is a rung of the theme's `ladder`, drawn
with `ui.Mark(t, "new")` (brand.md §4). A row in progress takes `ui.Busy(t, frame)`: the
theme's spinner when its frames are one cell, else the still partial glyph (in unicode
and ASCII alike), never a multi-cell spinner that pushes the name out of line; the
clock for such a row runs only when `ui.BusyAnimates(t)`. The label or a tag beside it
says the state in words.

## Progress, Sparkline, Meter — `ui.Progress`, `ui.Sparkline`

- Progress: `━━━━╸╌╌╌` with the fill in a short gradient, then the fraction and what is
  progressing ("3/5 pods"). Indeterminate work gets a spinner, not a fake bar.
- A reading (a meter, a rate, a percentage) past its warning level carries the theme's
  warn mark beside the value (`ui.Mark(t, "warn")`, `t.G.Warn`: `▲`, ascii `!`), and
  past its error level the fail mark, not only amber or red: at 16 colours amber is often
  red's slot and under NO_COLOR it is the text colour, so the band is the mark's to say.
  The `ladder` frame shows the marks a reading can carry.
- Sparkline: `▁▂▃▅▇` over a fixed column, coloured by state, with a flat series kept
  flat. Always print the current value beside it. A warning's ends in the warn mark
  (`ui.StateSparkline`: `▁▃▅▇█ ▲`): at 16 colours a warm brand's yellow and the
  warning's are one hue, so bars in yellow alone read as the selection. A sparkline taller than one row grows
  upward from the label's row: label, value and the bar's base share the bottom row,
  the rows above hold the peaks, so a two-row graph never splits the label from the
  bar it names (a `▂▆▆▂` fragment alone on a row reads as noise).

## Empty, Loading, Error — `ui.Empty`

Every list and pane designs all three:

- **Empty**: a quiet glyph, what is missing, why (in context: "checkout-api has been
  failing for 12 min"), and the one action that fixes it, as a key.
- **Loading**: a skeleton of the real layout with dim placeholders, or one spinner with
  a subject. Never a blank screen.
- **Error**: what failed, against what, the likely cause, and the key to retry. Keep the
  rest of the UI usable.

## Tree — `ui.Tree`

Rounded rails `├─ ╰─` in subtle, children indented so wrapped lines keep the rail. Use it
for steps, nested agents and dependencies.

## Splash

See `cell-art.md`. Mark, wordmark, spinner with subject, version bottom-left, quit hint
bottom-right, brand field behind with a clearing. It is shown only while something loads.

## Too Small — `ui.TooSmall(t, w, h, minW, minH, bindings)`

One centred line with the actual and the needed size, then the keys of the `toosmall`
row of the action table (`m.footerBindings()` while the size is below the minimum),
never a key typed into the screen: `TestHelpIsHandled` presses every one. Nothing else.
