// Package palette is the engine behind the palette command: derive a dark and a light
// role set from one to three brand colours in OKLCH, and check any role set's contrast.
// It is a port of scripts/palette.py and produces the same bytes (see the testdata
// goldens). It differs on purpose in six places:
//
//   - derive's on-colours come from the kit's theme.OnInk: on a mid-tone fill where
//     neither ink clears the check's floor (palette.py emitted a failing palette, e.g.
//     for #2E8B57), the ink deepens toward black or white along its hue, so with the
//     backgrounds it picks, derive's output always passes its own check and the brand's
//     primary keeps its hex;
//   - check takes a missing on-colour from the kit's engine (theme.Resolve) rather than
//     re-deriving it, so a theme without bg or fg is checked against the engine's default
//     inks (#15171C/#D8DEE9 dark) where palette.py used #000000/#ffffff;
//   - options also take "--opt=value" (palette.py read "--bg=#000" as an option named
//     "bg=#000" and swallowed the next argument);
//   - the swatch hint names the Go cellshot, not cellshot.py;
//   - bad input (a missing file, a role that is not a hex colour, a missing option value)
//     exits 2 with one line on stderr instead of a traceback (exit 1); check validates
//     every role it may read up front, where palette.py failed only on the one it reached;
//   - -h prints the command's Go doc comment.
package palette

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"tuikit/theme"
	"tuitools/internal/oklch"
	"tuitools/internal/pyfmt"
)

// Mode is one named role set ("dark" or "light"), role name to hex.
type Mode struct {
	Name  string
	Roles map[string]string
}

// pymax and pymin are Python's two-argument max and min: the first wins a tie.
func pymax(a, b float64) float64 {
	if b > a {
		return b
	}
	return a
}

func pymin(a, b float64) float64 {
	if b < a {
		return b
	}
	return a
}

// tuneL moves lightness from l0 in dir (+1 lighter, -1 darker) in steps of 0.005 until
// the colour's contrast against bg reaches target.
func tuneL(l0, c, h float64, bg string, target, dir float64) float64 {
	l := l0
	for range 200 {
		if oklch.Contrast(oklch.FromLCH(l, c, h), bg) >= target {
			return l
		}
		l = pymax(0.0, pymin(1.0, l+0.005*dir))
		if l == 0 || l == 1 {
			break
		}
	}
	return l
}

// stepL is l moved by `by` in direction d (+1 lighter, -1 darker), or the other way
// where that would leave the lightness a colour can hold.
func stepL(l, d, by float64) float64 {
	if n := l + d*by; 0.08 <= n && n <= 0.95 {
		return n
	}
	return pymax(0, pymin(1, l-d*by))
}

// nearPrimaryMax is how close, in OKLab a/b (hue and chroma, lightness aside), a
// warning may sit to primary before check warns: an amber brand's warning is the brand
// colour a shade brighter, and a warning row reads as a focused one. The kit's themes
// keep 0.107 or more (ferrous light); derive's warning on an amber primary is 0.03 off.
const nearPrimaryMax = 0.06

// NearPrimary reports the OKLab a/b distance between a role set's warning and primary,
// and whether it is under nearPrimaryMax. Both must be present, differ, and carry a hue
// (chroma 0.04 or more): two near-greys are close in a/b whatever their hues.
func NearPrimary(r map[string]string) (float64, bool) {
	w, p := r["warning"], r["primary"]
	if w == "" || p == "" || !oklch.Valid(w) || !oklch.Valid(p) || strings.EqualFold(w, p) {
		return 0, false
	}
	_, cw, _ := oklch.LCH(w)
	_, cp, _ := oklch.LCH(p)
	_, a1, b1 := oklch.ToOKLab(oklch.MustRGB(w))
	_, a2, b2 := oklch.ToOKLab(oklch.MustRGB(p))
	d := math.Sqrt((a1-a2)*(a1-a2) + (b1-b2)*(b1-b2))
	return d, cw >= 0.04 && cp >= 0.04 && d < nearPrimaryMax
}

var statusHues = []struct {
	role string
	hue  float64
}{{"success", 150}, {"error", 22}, {"warning", 85}, {"info", 240}}

// Derive builds the dark and light role sets. secondary, accent, bg and lightBg may be
// empty: the secondary then sits 40° from the primary, the accent is a light low-chroma
// tint of it, and each background is a near-neutral tinted toward its hue. Every input
// must be a valid hex colour (see oklch.Valid).
func Derive(primary, secondary, accent, bg, lightBg string, tint float64) []Mode {
	lp, cp, hp := oklch.LCH(primary)
	oneHue := secondary == ""
	ls, cs, hs := lp, cp, hp
	if !oneHue {
		ls, cs, hs = oklch.LCH(secondary)
	}
	la, ca, ha := 0.88, pymin(0.08, cp*0.5), hp
	if accent != "" {
		la, ca, ha = oklch.LCH(accent)
	}
	var out []Mode
	for _, mode := range []string{"dark", "light"} {
		dark := mode == "dark"
		b := lightBg
		if dark {
			b = bg
		}
		if b == "" {
			if dark {
				b = oklch.FromLCH(0.20, tint*1.2, hp)
			} else {
				b = oklch.FromLCH(0.985, tint*0.6, hp)
			}
		}
		lb, cb, hb := oklch.LCH(b)
		d := 1.0 // direction "away from the background"
		if !dark {
			d = -1
		}
		nc := pymax(tint, pymin(cb, 0.04)) // neutral chroma, tinted toward the brand hue
		hn := hp                           // neutrals lean toward a coloured background's own hue
		if cb > 0.012 {
			hn = hb
		}
		r := map[string]string{"bg": b}
		r["bgSubtle"] = oklch.FromLCH(lb+d*0.075, nc*1.4, hn)
		r["bgOverlay"] = oklch.FromLCH(lb+d*0.13, nc*1.6, hn)
		r["bgPanel"] = oklch.FromLCH(lb+d*0.035, nc*1.2, hn)
		r["border"] = oklch.FromLCH(tuneL(lb+d*0.07, nc*1.6, hn, b, 1.5, d), nc*1.6, hn) // visible but quiet: the check warns under 1.5
		r["fgSubtle"] = oklch.FromLCH(tuneL(lb+d*0.25, nc*1.8, hn, b, 2.7, d), nc*1.8, hn)
		r["fgMuted"] = oklch.FromLCH(tuneL(lb+d*0.35, nc*1.6, hn, b, 4.6, d), nc*1.6, hn)
		fgT, softT := 11.0, 8.0
		if !dark {
			fgT, softT = 12, 9
		}
		r["fg"] = oklch.FromLCH(tuneL(lb+d*0.62, nc*0.8, hn, b, fgT, d), nc*0.8, hn)
		r["fgSoft"] = oklch.FromLCH(tuneL(lb+d*0.52, nc, hn, b, softT, d), nc, hn)

		role := func(l, c, h, target float64) string {
			l0 := l
			if !dark {
				l0 = pymin(l, 0.62)
			}
			return oklch.FromLCH(tuneL(l0, c, h, b, target, d), c, h)
		}
		r["primary"] = role(lp, cp, hp, 3.5)
		if oneHue {
			// one brand hue: secondary and tertiary are lightness steps of it, away from
			// the page (back toward it where the step would leave the gamut), not a hue
			// the brand never chose
			lp2, _, _ := oklch.LCH(r["primary"])
			r["secondary"] = role(stepL(lp2, d, 0.10), cs, hs, 3.5)
			r["tertiary"] = role(stepL(lp2, d, 0.20), pymin(cp, 0.14), hp, 4.5)
			// where the lightness step lands on the primary itself (a primary tuned to
			// the end of the ramp on a mid-grey page), step chroma instead; and where
			// that end is white, where chroma has no room, step back toward the page at
			// the check's own floor for secondary (3.0, not derive's 3.5)
			if r["secondary"] == r["primary"] {
				r["secondary"] = role(lp2, cs*0.5, hs, 3.5)
			}
			if r["secondary"] == r["primary"] {
				r["secondary"] = role(stepL(lp2, -d, 0.10), cs, hs, 3.0)
			}
			if r["tertiary"] == r["primary"] || r["tertiary"] == r["secondary"] {
				r["tertiary"] = role(lp2, pymin(cp, 0.14)*0.25, hp, 4.5)
			}
		} else {
			r["secondary"] = role(ls, cs, hs, 3.5)
			if dark {
				r["tertiary"] = role(pymax(lp, 0.78), pymin(cp, 0.14), oklch.Mod(hp+160, 360), 4.5)
			} else {
				r["tertiary"] = role(0.5, pymin(cp, 0.14), oklch.Mod(hp+160, 360), 4.5)
			}
		}
		if dark {
			r["accent"] = role(la, ca, ha, 4.5)
		} else {
			r["accent"] = role(0.42, pymax(ca, 0.09), ha, 4.5)
		}
		r["borderFocus"] = r["primary"]
		bandL := 0.50
		if dark {
			bandL = pymax(0.68, pymin(0.80, lp))
		}
		bandC := pymax(0.13, pymin(0.19, cp))
		for _, s := range statusHues {
			l, target := bandL, 4.5
			if s.role == "warning" {
				l = 0.55
				if dark {
					l = 0.84
				}
			}
			if s.role == "info" {
				target = 3.5
			}
			r[s.role] = role(l, bandC, s.hue, target)
		}
		lightInk, darkInk := oklch.FromLCH(0.985, nc*0.4, hn), oklch.FromLCH(0.18, nc*1.5, hn)
		on := func(role, fill string) string { // text on a filled role: the engine's ink (theme.OnInk)
			c := theme.OnInk(oklch.Color(r[fill]), oklch.Color(lightInk), oklch.Color(darkInk), onFloor(role))
			switch {
			case same(c, oklch.Color(lightInk)):
				return lightInk
			case same(c, oklch.Color(darkInk)):
				return darkInk
			}
			return hexOf(c) // neither ink clears the floor: one pushed toward black or white
		}
		r["onPrimary"], r["onSecondary"], r["onWarning"] = on("onPrimary", "primary"), on("onSecondary", "secondary"), on("onWarning", "warning")
		r["onError"], r["onSuccess"] = on("onError", "error"), on("onSuccess", "success")
		r["fgSelected"] = r["onPrimary"]
		out = append(out, Mode{mode, r})
	}
	return out
}

// Check is one contrast requirement: role a against role b, within [min, max]; a pair
// in warnFloors passes with a warning under that floor.
type Check struct {
	A, B     string
	Min, Max float64
	want     string // the range as palette.py prints it
}

// warnFloor is the floor a pair should clear, under which check warns, and what reads
// wrong under it.
type warnFloor struct {
	floor float64
	why   string
}

// warnFloors are the pairs that pass with a warning under a floor above their Min: a
// rule is half the idiom (Law 2); under 1.3 nobody sees it (1.18 passed the old 1.14
// floor with every rule invisible on the sheet), under 1.5 it is barely there. The
// same numbers on a light page, where 1.5:1 is the same ~15 L* step.
var warnFloors = map[[2]string]warnFloor{
	{"border", "bg"}: {1.5, "the section, table and footer rules are barely there; a rule reads at 1.5–1.9"},
}

// Checks are the contrast requirements, in report order.
var Checks = []Check{
	{"fg", "bg", 7.0, 0, "≥7.0"}, {"fgMuted", "bg", 4.5, 0, "≥4.5"}, {"fgSubtle", "bg", 2.6, 0, "≥2.6"},
	{"fg", "bgSubtle", 4.5, 0, "≥4.5"}, {"fgMuted", "bgSubtle", 3.0, 0, "≥3.0"},
	{"primary", "bg", 3.5, 0, "≥3.5"}, {"secondary", "bg", 3.0, 0, "≥3.0"}, {"tertiary", "bg", 4.5, 0, "≥4.5"},
	{"accent", "bg", 4.5, 0, "≥4.5"}, {"success", "bg", 3.5, 0, "≥3.5"}, {"error", "bg", 3.5, 0, "≥3.5"},
	{"warning", "bg", 3.5, 0, "≥3.5"}, {"info", "bg", 3.0, 0, "≥3.0"}, {"border", "bg", 1.3, 1.9, "≥1.3 ≤1.9"}, // and a warning under 1.5 (warnFloors)
	{"bgSubtle", "bg", 1.1, 2.2, "≥1.1 ≤2.2"}, {"onPrimary", "primary", 4.5, 0, "≥4.5"}, {"onSecondary", "secondary", 3.5, 0, "≥3.5"},
	{"onWarning", "warning", 4.5, 0, "≥4.5"}, {"onError", "error", 3.5, 0, "≥3.5"}, {"onSuccess", "success", 3.5, 0, "≥3.5"},
	{"fgSelected", "primary", 3.5, 0, "≥3.5"},
}

// onFloor is the contrast Checks asks of an on-colour on its fill.
func onFloor(on string) float64 {
	for _, f := range onFills {
		if f.on != on {
			continue
		}
		for _, c := range Checks {
			if c.A == on && c.B == f.fill {
				return c.Min
			}
		}
	}
	panic("palette: no check for " + on)
}

// ExpressionChecks are the pairs that exist only because of an expression choice
// (mirroring kit/ui): a "bar" footer draws the key hints in fgSubtle on bgSubtle, so
// that pair must clear tuilint's 2.3 floor.
func ExpressionChecks(expressions map[string]any) []Check {
	var extra []Check
	if footer, _ := expressions["footer"].(string); footer == "bar" {
		extra = append(extra, Check{"fgSubtle", "bgSubtle", 2.3, 0, "≥2.3"})
	}
	return extra
}

// ExpressionSkips are the Checks an expression choice takes off screen: under
// "focus": "fg" no selected row is a primary fill (the fill selection is drawn in fg),
// so fgSelected on primary is never drawn. primary on bg stays (primary is still the
// state colour the theme reserved it for) and so do the on-primary fills (a bar header,
// a filled tag).
func ExpressionSkips(expressions map[string]any) map[[2]string]bool {
	skip := map[[2]string]bool{}
	if focus, _ := expressions["focus"].(string); focus == "fg" {
		skip[[2]string{"fgSelected", "primary"}] = true
	}
	return skip
}

// Report checks one role set and returns the number of failures and the report text.
// accept maps "<label>.<role>" to the reason a theme gives in writing for a deviation:
// a failing check on that role is reported as ACCEPT and not counted.
func Report(roles map[string]string, label string, accept map[string]string, extra ...Check) (int, string) {
	return ReportSkipping(roles, label, accept, nil, extra...)
}

// ReportSkipping is Report without the Checks in skip (see ExpressionSkips).
func ReportSkipping(roles map[string]string, label string, accept map[string]string, skip map[[2]string]bool, extra ...Check) (int, string) {
	bad := 0
	lines := []string{"  " + label}
	for _, c := range append(append([]Check(nil), Checks...), extra...) {
		if skip[[2]string{c.A, c.B}] {
			continue
		}
		a, okA := roles[c.A]
		b, okB := roles[c.B]
		if !okA || !okB {
			continue
		}
		ratio := oklch.Contrast(a, b)
		ok := ratio >= c.Min && (c.Max == 0 || ratio <= c.Max)
		why := accept[label+"."+c.A]
		mark := "ok "
		if w, has := warnFloors[[2]string{c.A, c.B}]; ok && has && ratio < w.floor {
			mark = "warn"
		}
		if !ok {
			mark = "FAIL"
			if why != "" {
				mark = "ACCEPT"
			} else {
				bad++
			}
		}
		line := fmt.Sprintf("    %-4s %-12s on %-9s %5.2f  (want %s)  %s / %s", mark, c.A, c.B, ratio, c.want, a, b)
		if why != "" && !ok {
			line += "  — " + why
		}
		lines = append(lines, line)
	}
	return bad, strings.Join(lines, "\n")
}

// Warnings are the checks a role set passes under their Warn floor (the border under
// 1.5:1), one line each for stderr, as mode names the set.
func Warnings(roles map[string]string, mode string) []string {
	var out []string
	for _, c := range Checks {
		w, has := warnFloors[[2]string{c.A, c.B}]
		a, okA := roles[c.A]
		b, okB := roles[c.B]
		if !has || !okA || !okB {
			continue
		}
		if ratio := oklch.Contrast(a, b); ratio >= c.Min && ratio < w.floor {
			out = append(out, fmt.Sprintf("%s: %s %s on %s %s is %.2f:1, under %.1f: %s", mode, c.A, a, c.B, b, ratio, w.floor, w.why))
		}
	}
	return out
}

// referenced are the roles a check reads: every role in Checks plus the inks the
// engine may put on a fill.
var referenced = func() map[string]bool {
	m := map[string]bool{"white": true}
	for _, c := range Checks {
		m[c.A], m[c.B] = true, true
	}
	return m
}()

var onFills = []struct{ on, fill string }{
	{"onPrimary", "primary"}, {"onSecondary", "secondary"}, {"onWarning", "warning"},
	{"onError", "error"}, {"onSuccess", "success"},
}

// Pin16 is the 16-colour index to pin a state role of colour hex to when it shares one
// with another state, among the colours no state role draws: a status role's own slot
// first (warning yellow, success green, error red, info blue or cyan; bright, then
// normal, on a dark page), since the terminal's palette keeps that meaning; else the
// nearest in OKLCH hue whose other brightness no state role draws either (10 beside a
// success on 2 is a different slot and the same reading). Bright (9-14) on a dark page,
// normal (1-6) on a light one, where terminal palettes put their legible hues. The greys
// (0, 7, 8, 15) carry no state and are never offered. A warning takes yellow even when
// both are held: on any other hue it reads as another state, and its glyph tells it from
// what shares the slot.
func Pin16(role, hex string, dark bool, used map[int]bool) int {
	shift := 0
	if dark {
		shift = 8
	}
	for _, i := range ownSlot[role] {
		for _, j := range []int{i + shift, i} { // on a light page only the normal one reads
			if !used[j] {
				return j
			}
		}
	}
	if role == "warning" {
		return 3 + shift // the warning never leaves yellow: on green it reads as success, on cyan as info; its glyph tells it from what shares the slot
	}
	_, _, h := oklch.LCH(hex)
	for _, base := range []int{shift, 8 - shift} {
		best, bestD := -1, 361.0
		for i := base + 1; i <= base+6; i++ {
			if used[i] || used[(i+8)%16] {
				continue
			}
			_, _, hi := oklch.LCH(hexOf(ansi.BasicColor(i)))
			d := math.Abs(h - hi)
			if d > 180 {
				d = 360 - d
			}
			if d < bestD {
				best, bestD = i, d
			}
		}
		if best >= 0 {
			return best
		}
	}
	return 3 + shift
}

// ownSlot is each status role's slot among the normal colours 1-6 (red, green,
// yellow, blue, magenta, cyan).
var ownSlot = map[string][]int{"error": {1}, "success": {2}, "warning": {3}, "info": {4, 6}}

func same(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	r1, g1, b1, a1 := a.RGBA()
	r2, g2, b2, a2 := b.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}

func hexOf(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
}

// FillOnColours adds the on-colours a theme leaves out, taking them from the kit's own
// engine (theme.Resolve), so the check sees the inks the app will draw. An on-colour is
// filled only when its fill role is present; fgSelected defaults to onPrimary. The ink
// is named by the role string it came from (white, bg, fg) where one matches.
func FillOnColours(r map[string]string, dark bool) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	var roles theme.Roles
	if err := json.Unmarshal(b, &roles); err != nil {
		return err
	}
	mode := "dark"
	if !dark {
		mode = "light"
	}
	p := theme.Resolve(theme.File{Roles: map[string]theme.Roles{mode: roles}}, dark, false).P
	engine := map[string]color.Color{
		"onPrimary": p.OnPrimary, "onSecondary": p.OnSecondary, "onWarning": p.OnWarning,
		"onError": p.OnError, "onSuccess": p.OnSuccess,
	}
	name := func(c color.Color) string {
		for _, k := range []string{"white", "bg", "fg"} {
			if s, ok := r[k]; ok && s != "" && same(oklch.Color(s), c) {
				return s
			}
		}
		return hexOf(c)
	}
	for _, f := range onFills {
		if _, ok := r[f.on]; ok {
			continue
		}
		if _, ok := r[f.fill]; !ok {
			continue
		}
		r[f.on] = name(engine[f.on])
	}
	if _, ok := r["fgSelected"]; !ok {
		if v, ok := r["onPrimary"]; ok {
			r["fgSelected"] = v
		}
	}
	return nil
}

// Swatch renders the role sets as a 64-column .ansi file: each hue as a block and a
// sample line, the text ramp, a border rule, and buttons on their fills.
func Swatch(modes []Mode) string {
	sgr := func(code int, h string) string {
		c := oklch.MustRGB(h)
		return fmt.Sprintf("\x1b[%d;2;%d;%d;%dm", code, oklch.Round(c[0]*255), oklch.Round(c[1]*255), oklch.Round(c[2]*255))
	}
	fg := func(h string) string { return sgr(38, h) }
	bgc := func(h string) string { return sgr(48, h) }
	const w = 64
	var rows []string
	for _, m := range modes {
		r := m.Roles
		bg := bgc(r["bg"])
		line := func(s string, vis int) string {
			return bg + s + bg + strings.Repeat(" ", max(0, w-vis)) + "\x1b[0m"
		}
		rows = append(rows, line(fg(r["fgMuted"])+" "+m.Name, utf8.RuneCountInString(m.Name)+1))
		for _, k := range []string{"primary", "secondary", "tertiary", "accent", "success", "error", "warning", "info"} {
			txt := " ██ " + pyfmt.Ljust(k, 10) + " " + r[k] + "  "
			sample := "The quick brown fox"
			rows = append(rows, line(fg(r[k])+txt+sample, utf8.RuneCountInString(txt)+len(sample)))
		}
		for _, k := range []string{"fg", "fgMuted", "fgSubtle"} {
			txt := " Aa " + pyfmt.Ljust(k, 10) + " " + r[k] + "  "
			sample := "body text at this level"
			rows = append(rows, line(fg(r[k])+txt+sample, utf8.RuneCountInString(txt)+len(sample)))
		}
		rule := " " + fg(r["border"]) + strings.Repeat("─", 40) + fg(r["fgSubtle"]) + " border"
		rows = append(rows, line(rule, 48))
		pill := " " + bgc(r["secondary"]) + fg(r["onSecondary"]) + "  Save  " + bg + " " + bgc(r["bgSubtle"]) + fg(r["fg"]) +
			"  Cancel  " + bg + "  " + bgc(r["primary"]) + fg(r["onPrimary"]) + " selected row " + bg
		rows = append(rows, line(pill, 1+8+1+10+2+14))
		rows = append(rows, line("", 0))
	}
	return strings.Join(rows, "\n") + "\n"
}

func sortedRoles(r map[string]string) []string {
	keys := make([]string, 0, len(r))
	for k := range r {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// JSON renders the derive document as palette.py's json.dumps(indent=1) did:
// {"name", "roles": {mode: {role: hex}}, "ansi256": {mode: {role: index}}}, roles
// sorted, then "ansi16": {mode: {role: index}} when pins is not empty (Pins16): the
// theme contract's own key, so the block is copied with the roles.
func JSON(name string, modes []Mode, pins map[string]map[string]int) string {
	var b strings.Builder
	b.WriteString("{\n \"name\": " + pyfmt.QuoteJSON(name) + ",\n")
	section := func(key string, value func(string) string, last bool) {
		b.WriteString(" " + pyfmt.QuoteJSON(key) + ": {\n")
		for i, m := range modes {
			b.WriteString("  " + pyfmt.QuoteJSON(m.Name) + ": {\n")
			keys := sortedRoles(m.Roles)
			for j, k := range keys {
				b.WriteString("   " + pyfmt.QuoteJSON(k) + ": " + value(m.Roles[k]))
				if j < len(keys)-1 {
					b.WriteByte(',')
				}
				b.WriteByte('\n')
			}
			b.WriteString("  }")
			if i < len(modes)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString(" }")
		if !last {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	section("roles", pyfmt.QuoteJSON, false)
	section("ansi256", func(h string) string { return fmt.Sprint(oklch.Xterm256(h)) }, len(pins) == 0)
	if len(pins) > 0 {
		b.WriteString(" \"ansi16\": {\n")
		var ms []string
		for _, m := range modes {
			if len(pins[m.Name]) > 0 {
				ms = append(ms, m.Name)
			}
		}
		for i, m := range ms {
			b.WriteString("  " + pyfmt.QuoteJSON(m) + ": {\n")
			keys := sortedRoles16(pins[m])
			for j, k := range keys {
				b.WriteString("   " + pyfmt.QuoteJSON(k) + ": " + fmt.Sprint(pins[m][k]))
				if j < len(keys)-1 {
					b.WriteByte(',')
				}
				b.WriteByte('\n')
			}
			b.WriteString("  }")
			if i < len(ms)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString(" }\n")
	}
	b.WriteString("}")
	return b.String()
}
