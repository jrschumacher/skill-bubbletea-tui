// Palette derives a terminal role palette from brand colours, and checks one.
//
// Brand colours are measured from the brand (logo, site CSS, style guide), never invented.
// This turns 1–3 of them into the full role set a TUI needs, in OKLCH so lightness steps
// are perceptual, with neutrals tinted toward the brand hue (the single biggest reason a
// palette reads "designed" instead of "default grey"), status colours pulled into the
// brand's lightness/chroma band so they sit with it, and every text role checked for
// contrast against the background it will actually sit on.
//
//	palette derive "#6B50FF" "#FF60FF" [--accent "#E8FE96"] [--bg "#201F26"] [--light-bg "#FBFAF7"]
//	               [--tint 0.012] [--name crush] [--out theme-roles.json] [--swatch swatch.ansi]
//	palette check theme.json        contrast report for an existing theme's roles (exit 1 on fail)
//	                                a theme may accept a deviation in writing:
//	                                "contrastAccept": {"dark.primary": "the brand's own hex, used as a fill"}
//
// `derive` prints the roles as JSON (`{"dark": {...}, "light": {...}}`, the theme contract's
// shape), a contrast table, the nearest xterm-256 index for each role (for the
// ANSI256 fallback), and the "ansi16" pins its own check would ask for (below), so the
// first check of the theme prints no 16-colour pin. With one hex, secondary and tertiary are
// lightness steps of it (accent its pale tint), not a hue 40° off and a complement the
// brand never chose; with two, the second is the brand's and tertiary a complement.
// `--swatch` writes an .ansi file you can render with cellshot.
// Options take their value as the next argument or after "=".
//
// `check` fills the on-colours a theme leaves out exactly as the kit's engine does
// (tuikit/theme), so it checks the inks the app will draw. It reads role colours by the
// engine's rule too: "#RGB" or "#RRGGBB", nothing else. Lip Gloss reads "6B50FF" (no #)
// as an ANSI index or no colour, and "#6B50FFAA" as no colour, so those exit 2.
// It exits 2 on the theme's glyphs too (theme.CheckGlyphs): a glyph with no ASCII twin
// (in glyphs.ascii or the kit's map), a twin that isn't ASCII, or two marks of one
// meaning (done/current/todo/pending/partial/warn/fail, radio on/off, check on/off,
// thumb/track, focus/blur bar) that share a twin under -ascii, or a spinner frame that
// is, or whose cells hold, a status mark, a bar or the caret (a "···" spinner beside a
// "·" pending), or a blurBar that is the focusBar; and under selection "caret", a prompt
// that is the cursor or draws as its twin (theme.CheckSelectionGlyphs: a filter input
// above a caret-selected list would show one glyph twice). And on its expressions
// (theme.CheckExpressions): a key the kit doesn't read or a value it doesn't know, which
// theme.Load draws as the default silently ("dialog": "rule" for "rules"), named with
// the values the kit knows. A key under roles.<mode>, glyphs or wordmark the kit doesn't
// read ("primay", "focusbar": theme.Load drops it and the role falls back to the derived
// colour) and a wordmark.style it doesn't know exit 2 the same way (theme.CheckKeys).
//
// It warns, on stderr, when a mode's warning sits beside its primary in hue and chroma
// (OKLab a/b distance under 0.06: an amber brand's warning a shade brighter than its
// amber), since a warning row then reads as a focused one; derive warns the same.
//
// It reports, on stderr, state roles that land on one of the 16 colours (-profile ansi)
// after the theme's "ansi16" pins, each with the pin that separates them:
//
//	"ansi16": {"dark": {"primary": 11, "warning": 3}, "light": {"primary": 0}}
//
// Red (1 or 9) is error's. It exits 1 on error off red (a grey error beside a red
// warning reads reversed), and under "focus": "fg", where primary is a state, on primary
// on red or on another state role's colour (amber = new must not read as error). It warns
// on primary on red under "focus": "primary" (focus reads as an error), on warning,
// success or info on red, and on two of those on one colour: their glyphs and words still
// differ. The neutrals (bg, fg, fgMuted, fgSubtle, their steps, border, and borderFocus,
// the dialog's edge) draw on the four greys there (the kit assigns them: page 0 or 15,
// text the opposite; a dark page keeps three text levels, fgMuted 7 and fgSubtle 8; a
// light page keeps two, fgMuted and fgSubtle both 8, since ANSI 7 on a white page is
// below every floor and fgSubtle is never the only carrier of meaning; a rule on 8 or 7
// beside the page, the dialog's edge a step up on 7 or 8), so only a pin moves one: a
// neutral on red exits 1 (hints read as errors, a ruled dialog's edge as an error's), on
// another hue or two text levels on one grey it warns, each with the grey to pin
// instead; borderFocus pinned onto primary's own slot, the kit's truecolour default, is
// not reported. A pin that names no role, no mode or no colour 0-15 exits 2, as
// theme.Load does.
//
// The warning keeps yellow (11, then 3): on green 2/10, success's slot, it exits 1 (a
// warning reads as OK), and on blue or cyan, info's, it warns; its pin is yellow, even
// beside a brand that holds the other one. A warm brand (OKLCH 25°–85°: a vermilion, an
// orange, a copper, an amber) has no slot of its own: red is error's, yellow the
// warning's, magenta the pink an orange is not. On a dark page it takes bright yellow 11
// and the warning the other yellow, 3, told apart by the warning's glyph (▲ !); on a
// light page it has no hue, pinned to fg's grey 0 (yellow 3 is 1.35:1 on white), and the
// kit draws focus as fg and bold, as under "focus": "fg". A yellow brand has no hue on a
// light page either. Each off its slot is a warning with the pin.
//
// A primary the theme pins to a hue is checked there, in cellshot's palette (VS Code's
// terminal, the one every sheet shows): primary as text on the page at 3:1, and
// fgSelected (unless "focus": "fg") and onPrimary on it at 4.5:1, each as the kit draws
// it. Under one, it exits 1: white on bright yellow is 1.17:1, which the truecolour check
// of the hex never sees.
//
// Any other suggested pin is a status role's own slot, else the nearest free hue, never
// red and never the other brightness of a slot another state draws (10 beside success's
// 2 reads as success). When the brand and a status role are both displaced (an amber and
// its warning, both on red), the brand takes its slot first and the status role moves. A
// brand that is a status role's own colour (a yellow on 3, or within 25° of yellow's
// hue) yields the slot, each mode for itself. It warns on a pin, the theme's or
// derive's, that moves a role more than 60° of OKLCH hue from its truecolour value while
// the tool would pin it nearer (a rose pinned to blue 12 while magenta is free), naming
// the hex, its hue, the slot and the nearer pin; a pin that is the nearest free hue,
// however far, is not reported.
//
// Thresholds (WCAG 2 contrast ratio against the role's background):
//
//	fg ≥ 7      fgMuted ≥ 4.5     fgSubtle ≥ 2.6 (hints only, never the only carrier of meaning)
//	primary, success, error, warning ≥ 3.5    secondary, info ≥ 3.0    tertiary, accent ≥ 4.5
//	border 1.3–1.9, and a warning under 1.5 (a rule nobody sees is half the idiom gone;
//	  the same numbers on a light page, where 1.5:1 is the same step in L*)
//	bgSubtle 1.1–2.2 (barely there, but there)
//	fg ≥ 4.5 and fgMuted ≥ 3.0 on bgSubtle too
//	onPrimary, onWarning ≥ 4.5 on their fill; onSecondary, onError, onSuccess, fgSelected ≥ 3.5
//	plus the pairs a theme's expressions put on screen: "footer": "bar" draws the key
//	hints (fgSubtle) on bgSubtle, so that pair must clear tuilint's 2.3 floor; and minus
//	the ones they take off: under "focus": "fg" no selection is a primary fill, so
//	fgSelected on primary isn't checked (primary on bg and onPrimary still are)
//
// derive aims higher than these floors (fgMuted 4.6, fg 11–12, accent and status 4.5,
// border 1.5).
// On a mid-tone fill where neither ink clears its floor, derive deepens the ink toward
// black or white along its hue (the engine does the same), so the brand keeps its hex.
//
// Exit status: 0 OK, 1 contrast problems, error off red, a state clash under focus "fg",
// a neutral on red, a warning on green or a pinned primary under its floors at 16
// colours; 2 usage or input errors.
package main

import (
	_ "embed"
	"os"

	"tuitools/internal/cmddoc"
	"tuitools/internal/palette"
)

//go:embed main.go
var src string

func main() {
	args := os.Args[1:]
	// `palette derive -h` and `palette check -h` are help too, not a hex colour or a
	// theme path: the package doc is the one -h text, for the bare command and each mode.
	if len(args) == 2 && (args[0] == "derive" || args[0] == "check") && (args[1] == "-h" || args[1] == "--help") {
		args = args[1:]
	}
	os.Exit(palette.Main(args, cmddoc.Text(src), os.Stdout, os.Stderr))
}
