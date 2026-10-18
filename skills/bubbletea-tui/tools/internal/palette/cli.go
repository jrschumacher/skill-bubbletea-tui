package palette

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"math"
	"os"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"tuikit/theme"
	"tuitools/internal/cellshot"
	"tuitools/internal/oklch"
	"tuitools/internal/pyfmt"
)

// Main runs the palette command on args (without the program name) and returns its
// exit status: 0 OK, 1 contrast problems, 2 usage or input errors. usage is printed for
// -h and for an unknown mode.
func Main(args []string, usage string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(stdout, usage)
		return 0
	}
	switch args[0] {
	case "check":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: palette check theme.json")
			return 2
		}
		return check(args[1], stdout, stderr)
	case "derive":
		return derive(args[1:], stdout, stderr)
	}
	fmt.Fprintln(stdout, usage)
	return 2
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, "palette:", err)
	return 2
}

// Check reads a theme file (or a bare roles object) and reports every mode's contrast.
func check(path string, stdout, stderr io.Writer) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return fail(stderr, err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return fail(stderr, fmt.Errorf("%s: %w", path, err))
	}
	// theme.CheckKeys: a role, glyph or wordmark key the kit doesn't read ("primay",
	// "focusbar") is dropped by theme.Load and the role falls back to the derived colour
	// with nothing to say why; this is the gate, as for an expressions key below
	if err := theme.CheckKeys(data); err != nil {
		return fail(stderr, fmt.Errorf("%s: %w", path, err))
	}
	// theme.CheckGlyphs: a glyph -ascii can't draw, a twin that isn't ASCII, or two
	// states that share a twin. theme.Load lets them through so an app always starts;
	// this is the gate
	if raw, ok := top["glyphs"]; ok {
		var g theme.Glyphs
		if err := json.Unmarshal(raw, &g); err != nil {
			return fail(stderr, fmt.Errorf("%s: glyphs: %w", path, err))
		}
		if err := theme.CheckGlyphs(g); err != nil {
			return fail(stderr, fmt.Errorf("%s: %w", path, err))
		}
	}
	// expressions: a key the kit doesn't read, or a value it doesn't know, renders the
	// default silently (theme.Load loads it so an app starts); this is the gate
	if raw, ok := top["expressions"]; ok {
		if err := checkExpressions(raw); err != nil {
			return fail(stderr, fmt.Errorf("%s: %w", path, err))
		}
	}
	// the theme as the engine reads it, for its ansi16 pins and the 16-colour clashes
	var tf theme.File
	_ = json.Unmarshal(data, &tf) // a shape error surfaces below, with the role that has it
	if _, ok := top["roles"]; !ok {
		tf.Roles = nil
		_ = json.Unmarshal(data, &tf.Roles)
	}
	if err := theme.CheckANSI16(tf); err != nil {
		return fail(stderr, fmt.Errorf("%s: %w", path, err))
	}
	// theme.CheckSelectionGlyphs: under selection "caret" a prompt that is the cursor
	// (or its twin) draws one glyph twice on a palette; theme.Load lets it through
	if err := theme.CheckSelectionGlyphs(tf); err != nil {
		return fail(stderr, fmt.Errorf("%s: %w", path, err))
	}
	accept := map[string]string{}
	if raw, ok := top["contrastAccept"]; ok {
		if v, err := pyfmt.Decode(raw); err == nil {
			if m, ok := v.(map[string]any); ok {
				for k, why := range m {
					if pyfmt.Truthy(why) {
						accept[k] = pyfmt.Str(why)
					}
				}
			}
		}
	}
	var extra []Check
	var skip map[[2]string]bool
	if raw, ok := top["expressions"]; ok {
		if v, err := pyfmt.Decode(raw); err == nil {
			if m, ok := v.(map[string]any); ok {
				extra, skip = ExpressionChecks(m), ExpressionSkips(m)
			}
		}
	}
	rolesRaw := json.RawMessage(data)
	if raw, ok := top["roles"]; ok {
		rolesRaw = raw
	}
	modes, err := pyfmt.Keys(rolesRaw)
	if err != nil {
		return fail(stderr, fmt.Errorf("%s: roles: %w", path, err))
	}
	var byMode map[string]json.RawMessage
	if err := json.Unmarshal(rolesRaw, &byMode); err != nil {
		return fail(stderr, fmt.Errorf("%s: roles: %w", path, err))
	}
	bad := 0
	for _, mode := range modes {
		v, err := pyfmt.Decode(byMode[mode])
		if err != nil {
			return fail(stderr, err)
		}
		obj, ok := v.(map[string]any)
		if !ok {
			continue
		}
		r := map[string]string{}
		for _, k := range sortedKeys(obj) {
			val := obj[k]
			s, isStr := val.(string)
			if isStr && s == "" {
				continue // derived by the engine, as theme.Load does with a missing role
			}
			if !referenced[k] && !roleKeys[k] {
				if isStr {
					r[k] = s
				}
				continue
			}
			if !isStr {
				return fail(stderr, fmt.Errorf("%s: %s.%s: %s is not a hex colour", path, mode, k, pyfmt.Repr(val)))
			}
			// The engine's rule (theme.Load), not oklch.Parse's looser one: a role Lip
			// Gloss would misread ("6B50FF", "#6B50FFAA") must not pass as OK.
			if err := theme.CheckHex(s); err != nil {
				return fail(stderr, fmt.Errorf("%s: %s.%s: %w", path, mode, k, err))
			}
			r[k] = s
		}
		if err := FillOnColours(r, mode != "light"); err != nil {
			return fail(stderr, err)
		}
		b, txt := ReportSkipping(r, mode, accept, skip, extra...)
		bad += b
		fmt.Fprintln(stdout, txt)
		for _, w := range Warnings(r, mode) {
			fmt.Fprintf(stderr, "palette: warning: %s: %s\n", path, w)
		}
		if d, near := NearPrimary(r); near {
			fmt.Fprintf(stderr, "palette: warning: %s: %s\n", path, nearPrimaryText(mode, r, d))
		}
	}
	clashes := Clashes16(tf, path, stderr)
	for _, x := range Inks16(tf) {
		clashes++
		fmt.Fprintf(stderr, "palette: %s: %s\n", path, x)
	}
	for _, x := range FarPins(tf) {
		fmt.Fprintf(stderr, "palette: warning: %s: %s\n", path, farPinText(x))
	}
	if clashes > 0 {
		fmt.Fprintf(stdout, "FAIL (%d contrast problems, %d problems at 16 colours)\n", bad, clashes)
		return 1
	}
	fmt.Fprintln(stdout, verdict(bad))
	if bad > 0 {
		return 1
	}
	return 0
}

// Finding16 is one thing a 16-colour terminal reads wrong in a theme, after its ansi16
// pins, with the pin that fixes it (Findings16).
type Finding16 struct {
	Mode, Role string
	Col        string // the role as the theme writes it, truecolour hex
	What       string // the finding, without the path or the pin
	Fail       bool   // check exits 1 on it
	Pin        int    // the ansi16 colour that fixes it
	kind       kind16
	fixed      bool // Pin is set when found (a warm brand's slot, WarmPin16), not chosen by Pin16
}

type kind16 int

const (
	red16   kind16 = iota // a state role on red, or error off it (theme.ANSI16Red)
	clash16               // two state roles on one colour (theme.ANSI16Clashes)
	hue16                 // a neutral on a hue (theme.ANSI16Neutrals)
	share16               // two levels of the text ramp on one grey (theme.ANSI16Neutrals)
	evict16               // primary on a status role's own slot while that role needs one
	warm16                // a warm brand off its 16-colour slot (WarmPin16): magenta, red, or yellow on a light page
	yield16               // warning on bright yellow 11 on a dark page, the slot a warm brand takes
	warn16                // warning off yellow: on green (success's), blue or cyan (info's)
)

// PinJSON is the ansi16 block that applies the finding's pin.
func (x Finding16) PinJSON() string {
	return fmt.Sprintf(`"ansi16": {%q: {%q: %d}}`, x.Mode, x.Role, x.Pin)
}

// Findings16 is how the theme reads at 16 colours, after its ansi16 pins, each finding
// with the pin that fixes it:
//
//   - error off red (theme.ANSI16Red): a grey error beside a red warning reverses them.
//     Fails.
//   - another state role on red, error's slot: it reads as an error. Fails for primary
//     under focus "fg" (the theme's one state hue); a warning for the rest, primary under
//     focus "primary" included (there focus and selection draw in error's red), since the
//     glyph and the word still differ. The kit's own default warning lands on red, so
//     failing it would fail every theme that leaves warning unset.
//   - two state roles on one colour (theme.ANSI16Clashes): under focus "fg" a clash with
//     primary fails, since the theme promised its hue means one state (amber = new); any
//     other is a warning, as the glyph and the word still tell the two apart.
//   - a neutral role on a hue (theme.ANSI16Neutrals): the engine puts every neutral on a
//     grey, the roles a dialog draws (border, borderFocus, bgPanel, bgOverlay) included,
//     so only a pin moves one off. On red it fails (hints read as errors, a dialog's edge
//     as an error's); on another hue it warns, except borderFocus pinned onto primary's
//     own slot, the kit's truecolour default.
//   - two levels of the text ramp on one grey (theme.ANSI16Neutrals): a warning; the pin
//     is the grey the engine would have used.
//   - primary sitting on the own slot of a status role that needs a pin (a yellow brand
//     natively on 3 while warning is on red): a warning that moves primary to the nearest
//     free hue, so warning takes its yellow instead of being sent to cyan.
//   - a warm brand (an orange, copper or amber, WarmPin16) anywhere but its slot: bright
//     yellow 11 on a dark page, fg's grey on a light one, where it has no hue. A warning;
//     on a dark page the warning then moves from 11 to the other yellow, 3.
//   - the warning off yellow: on green 2/10, success's slot, it reads as OK and fails; on
//     blue or cyan (4/12, 6/14), info's, it warns. Its pin is yellow, whatever else
//     holds it: the warning never leaves yellow, and its glyph (▲ !) tells it apart.
//
// Which slot reads well on the page is the terminal's palette, so a pin is a suggestion:
// a status role to its own slot, else the nearest free hue, never red for a non-error,
// never a colour another suggestion took nor the other brightness of one another state
// role draws (10 beside success's 2 is a different slot and the same reading); a neutral
// to its grey (theme.GreySlot16). Failures come first, then primary, then the status
// roles (PinOrder): a brand whose amber and warning both land on red keeps the yellow
// nearest its hue and the warning moves, since a brand drawn green beside success's
// green reads as a success. Only a brand natively on a status role's own slot (the
// yellow brand above) yields it, in that mode (a dark lime on 11 says nothing about the
// light amber), and the terminal's own order holds there (keepRank: error, success,
// warning, then the brand's). Each suggestion is taken before the next, so following
// them all never makes a new problem.
func Findings16(f theme.File) []Finding16 {
	type key struct{ mode, role string }
	fg := f.Expressions.Focus == "fg"
	var found []Finding16
	covered := map[key]bool{}
	for _, r := range theme.ANSI16Red(f) {
		covered[key{r.Mode, r.Role}] = true
		what := fmt.Sprintf("%s: %s %s draws as ANSI %d at 16 colours (-profile ansi), error's red, so it reads as an error there", r.Mode, r.Role, hexOf(r.Col), r.Index)
		switch {
		case r.Role == "error":
			what = fmt.Sprintf("%s: error %s draws as ANSI %d at 16 colours (-profile ansi), not red (1 or 9), so a red state beside it reads as the error", r.Mode, hexOf(r.Col), r.Index)
		case r.Role == "primary" && !fg:
			what = fmt.Sprintf("%s: primary %s draws as ANSI %d at 16 colours (-profile ansi), error's red, so focus and selection read as errors there", r.Mode, hexOf(r.Col), r.Index)
		}
		found = append(found, Finding16{Mode: r.Mode, Role: r.Role, Col: hexOf(r.Col), What: what, Fail: r.Role == "error" || (r.Role == "primary" && fg), kind: red16})
	}
	clash := map[key][]string{}
	col := map[key]string{}
	var order []key
	for _, c := range theme.ANSI16Clashes(f) {
		move, keep, colMove, colKeep := c.B, c.A, c.ColB, c.ColA
		if keepRank[c.B] < keepRank[c.A] {
			move, keep, colMove, colKeep = c.A, c.B, c.ColA, c.ColB
		}
		k := key{c.Mode, move}
		if covered[k] {
			continue // its red finding's pin moves it off this colour too
		}
		if _, seen := clash[k]; !seen {
			order = append(order, k)
			col[k] = hexOf(colMove)
		}
		clash[k] = append(clash[k], keep+" "+hexOf(colKeep))
	}
	for _, k := range order {
		others, verb := strings.Join(clash[k], " and "), "do"
		if len(clash[k]) == 1 {
			verb = "does"
		}
		withPrimary := k.role == "primary" || slices.ContainsFunc(clash[k], func(o string) bool { return strings.HasPrefix(o, "primary ") })
		what := fmt.Sprintf("%s: %s %s draws as ANSI %d at 16 colours (-profile ansi), as %s %s", k.mode, k.role, col[k], theme.ANSI16Index(f, k.mode, k.role), others, verb)
		found = append(found, Finding16{Mode: k.mode, Role: k.role, Col: col[k], What: what, Fail: fg && withPrimary, kind: clash16})
	}
	for _, n := range theme.ANSI16Neutrals(f) {
		if n.Role == "borderFocus" && n.Index == theme.ANSI16Index(f, n.Mode, "primary") && n.Index != 1 && n.Index != 9 {
			continue // the dialog's edge in the brand's own hue is the kit's truecolour default, pinned on purpose
		}
		x := Finding16{Mode: n.Mode, Role: n.Role, Col: hexOf(n.Col), kind: hue16}
		switch {
		case n.With != "":
			x.kind = share16
			x.What = fmt.Sprintf("%s: %s %s draws as ANSI %d at 16 colours (-profile ansi), as %s %s does, so %s reads as %s there",
				n.Mode, n.Role, x.Col, n.Index, n.With, roleHex(f, n.Mode, n.With), neutralText[n.Role], neutralText[n.With])
		case n.Index == 1 || n.Index == 9:
			x.Fail = true
			x.What = fmt.Sprintf("%s: %s %s draws as ANSI %d at 16 colours (-profile ansi), error's red, so %s reads as an error there", n.Mode, n.Role, x.Col, n.Index, neutralText[n.Role])
		default:
			x.What = fmt.Sprintf("%s: %s %s draws as ANSI %d at 16 colours (-profile ansi), a hue, so %s reads as a state there", n.Mode, n.Role, x.Col, n.Index, neutralText[n.Role])
		}
		found = append(found, x)
	}
	// the warning off yellow, its own slot in every terminal palette: on green it reads as
	// success, on blue or cyan as info (red is ANSI16Red's above)
	for _, mode := range []string{"dark", "light"} {
		if _, ok := f.Roles[mode]; !ok {
			continue
		}
		if covered[key{mode, "warning"}] {
			continue // on red: its red finding moves it, onto yellow
		}
		w := theme.ANSI16Index(f, mode, "warning")
		reads := map[int]string{2: "success's green", 10: "success's green", 4: "info's blue", 12: "info's blue", 6: "info's cyan", 14: "info's cyan"}[w]
		if reads == "" {
			continue
		}
		// a clash with success or info on that slot is this finding, said once
		found = slices.DeleteFunc(found, func(x Finding16) bool { return x.Mode == mode && x.Role == "warning" && x.kind == clash16 })
		tc := theme.Resolve(f, mode != "light", false).P
		state := "info"
		if w == 2 || w == 10 {
			state = "success"
		}
		found = append(found, Finding16{Mode: mode, Role: "warning", Col: hexOf(tc.Warning), Fail: state == "success", kind: warn16,
			What: fmt.Sprintf("%s: warning %s draws as ANSI %d at 16 colours (-profile ansi), %s, so a warning reads as %s there; the warning keeps yellow", mode, hexOf(tc.Warning), w, reads, state)})
	}
	has := map[key]bool{}
	for _, x := range found {
		has[key{x.Mode, x.Role}] = true
	}
	// a warm brand has no slot of its own (WarmPin16): it takes yellow 11 on a dark page
	// and no hue on a light one, before anything else is decided, so the evictions below
	// never move it and the warning's pin sees its yellow taken
	for _, mode := range []string{"dark", "light"} {
		want, ok := WarmPin16(f, mode)
		if !ok {
			continue
		}
		tc := theme.Resolve(f, mode != "light", false).P
		_, _, h := oklch.LCH(hexOf(tc.Primary))
		if at := theme.ANSI16Index(f, mode, "primary"); at != want {
			what := fmt.Sprintf("%s: primary %s (hue %.0f°, warm) draws as ANSI %d at 16 colours (-profile ansi), %s; a warm brand has no slot of its own there (red is error's, magenta the pink an orange is not), so it takes bright yellow and the warning the other yellow, told apart by the warning's glyph", mode, hexOf(tc.Primary), h, at, slotName(at))
			if mode == "light" {
				kind := "warm"
				if !warm(h) {
					kind = "yellow"
				}
				what = fmt.Sprintf("%s: primary %s (hue %.0f°, %s) draws as ANSI %d at 16 colours (-profile ansi), %s; on a light page a %s brand has no slot (yellow 3 is the warning's and 1.35:1 on white, cyan and green under 3:1, magenta the pink an orange is not), so it draws as fg, and the kit's focus as fg and bold", mode, hexOf(tc.Primary), h, kind, at, slotName(at), kind)
			}
			if i := slices.IndexFunc(found, func(x Finding16) bool { return x.Mode == mode && x.Role == "primary" }); i >= 0 {
				found[i].Pin, found[i].fixed = want, true // its red or clash finding: the warm slot fixes that too
			} else {
				found = append(found, Finding16{Mode: mode, Role: "primary", Col: hexOf(tc.Primary), What: what, kind: warm16, Pin: want, fixed: true})
				has[key{mode, "primary"}] = true
			}
		}
		if mode != "light" && !has[key{mode, "warning"}] && theme.ANSI16Index(f, mode, "warning") == want {
			found = append(found, Finding16{Mode: mode, Role: "warning", Col: hexOf(tc.Warning), kind: yield16,
				What: fmt.Sprintf("%s: warning %s draws as ANSI %d at 16 colours (-profile ansi), bright yellow, the slot warm primary %s takes; the warning keeps the other yellow and its glyph", mode, hexOf(tc.Warning), want, hexOf(tc.Primary))})
			has[key{mode, "warning"}] = true
		}
		// the brand's own steps (secondary, tertiary, accent) of a warm hue follow it off
		// error's red and magenta, and off yellow 3 on a light page: copper's secondary drew
		// the focused button in error's red, its accent every section title
		for _, role := range []string{"secondary", "tertiary", "accent"} {
			col := paletteHex(tc, role)
			_, c, hr := oklch.LCH(col)
			at := theme.ANSI16Index(f, mode, role)
			off := map[int]bool{1: true, 9: true, 5: true, 13: true, 3: mode == "light", 11: mode == "light"}
			if c < 0.04 || !(warm(hr) || mode == "light" && hueDiff(hr, yellowHue) <= ownHueMax) || at == want || !off[at] {
				continue
			}
			found = append(found, Finding16{Mode: mode, Role: role, Col: col, kind: warm16, Pin: want, fixed: true,
				What: fmt.Sprintf("%s: %s %s (hue %.0f°), a step of the warm brand, draws as ANSI %d at 16 colours (-profile ansi), %s, so the brand reads as %s there; it follows primary", mode, role, col, hr, at, slotName(at), map[bool]string{true: "an error", false: "another hue"}[at == 1 || at == 9])})
		}
	}
	// a status role whose own slot (warning 3/11, success 2/10, info 4/12) is held by
	// primary's native index: evict primary, since the terminal's palette owns that
	// meaning, rather than send the status role to the nearest free hue
	native := f
	native.ANSI16 = nil
	for _, x := range found {
		own, isStatus := ownSlot[x.Role]
		if !isStatus || x.kind == hue16 || x.kind == share16 || has[key{x.Mode, "primary"}] {
			continue
		}
		if _, warm := WarmPin16(f, x.Mode); warm {
			continue // a warm brand's yellow is its own on a dark page (and it has none on a light one)
		}
		shift := 0
		if x.Mode != "light" {
			shift = 8
		}
		p := theme.ANSI16Index(native, x.Mode, "primary") // where the brand lands by itself: a pin onto the slot is the theme's choice and holds
		for _, i := range own {
			if p == i+shift || p == i {
				tc := theme.Resolve(f, x.Mode != "light", false).P
				found = append(found, Finding16{Mode: x.Mode, Role: "primary", Col: hexOf(tc.Primary), kind: evict16,
					What: fmt.Sprintf("%s: primary %s draws as ANSI %d at 16 colours (-profile ansi), %s's own slot, which %s needs", x.Mode, hexOf(tc.Primary), p, x.Role, x.Role)})
				has[key{x.Mode, "primary"}] = true
				break
			}
		}
	}
	// failures first, then PinOrder: primary before the status roles when a slot near
	// its hue exists, unless the brand sits natively on one's own slot (evict16) and
	// yields it
	evicted := func(mode string) bool {
		return slices.ContainsFunc(found, func(x Finding16) bool { return x.kind == evict16 && x.Mode == mode })
	}
	rank := func(x Finding16) int {
		if x.kind == hue16 || x.kind == share16 {
			return len(keepRank)
		}
		return PinOrder(f, x.Mode, evicted(x.Mode))[x.Role]
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].Fail != found[j].Fail {
			return found[i].Fail
		}
		return rank(found[i]) < rank(found[j])
	})
	// a slot is taken by red, and by every state role that stays where it is; a role
	// with a finding is being moved, so its slot is free for the next
	used := map[string]map[int]bool{}
	for i, x := range found {
		if used[x.Mode] == nil {
			used[x.Mode] = map[int]bool{1: true, 9: true} // red is error's alone
			for _, r := range theme.StateRoles {
				if !has[key{x.Mode, r}] {
					used[x.Mode][theme.ANSI16Index(f, x.Mode, r)] = true
				}
			}
			for _, y := range found { // a warm brand's slot is held before any pin is chosen
				if y.Mode == x.Mode && y.fixed {
					used[x.Mode][y.Pin] = true
				}
			}
		}
		dark := x.Mode != "light"
		switch {
		case x.fixed:
		case x.kind == hue16 || x.kind == share16:
			found[i].Pin = greyPin(f, x.Mode, x.Role)
			continue
		case x.Role == "error":
			found[i].Pin = 1
			if dark {
				found[i].Pin = 9
			}
		default:
			found[i].Pin = Pin16(x.Role, x.Col, dark, used[x.Mode])
		}
		used[x.Mode][found[i].Pin] = true
	}
	return found
}

// neutralText is what each neutral role carries on screen, for a finding's sentence
// (singular, so "reads" follows each).
var neutralText = map[string]string{
	"bg": "the page", "bgPanel": "a panel", "bgSubtle": "the background step", "bgOverlay": "an overlay", "border": "every rule", "borderFocus": "the dialog's edge",
	"fg": "body text", "fgSoft": "prose", "fgMuted": "secondary text", "fgSubtle": "a hint",
}

// roleHex is a text ramp role of mode as the theme draws it at truecolour.
func roleHex(f theme.File, mode, role string) string {
	p := theme.Resolve(f, mode != "light", false).P
	return hexOf(map[string]color.Color{"fg": p.Fg, "fgMuted": p.FgMuted, "fgSubtle": p.FgSubtle}[role])
}

// greyPin is the grey to pin neutral role to in mode: its own (theme.GreySlot16), unless
// another level of the text ramp already draws there; then the other text grey, then
// fg's. The page's grey is never offered for text.
func greyPin(f theme.File, mode, role string) int {
	dark := mode != "light"
	own := theme.GreySlot16(role, dark)
	if !slices.Contains(theme.TextRamp, role) {
		return own
	}
	taken := map[int]bool{theme.GreySlot16("bg", dark): true}
	for _, other := range theme.TextRamp {
		if other != role {
			taken[theme.ANSI16Index(f, mode, other)] = true
		}
	}
	for _, n := range []int{own, theme.GreySlot16("fgMuted", dark), theme.GreySlot16("fgSubtle", dark), theme.GreySlot16("fg", dark)} {
		if !taken[n] {
			return n
		}
	}
	return own
}

// Ink16 is a pair a theme's primary pin draws below its floor at 16 colours (Inks16).
type Ink16 struct {
	Mode       string
	Role, On   string  // the ink and what it is drawn on: primary on bg, fgSelected or onPrimary on primary
	Hex        string  // the ink role as the theme writes it, truecolour hex
	Index, OnI int     // the 16-colour slots the two draw as, after the kit's own ink swap (theme.Ink)
	Ratio      float64 // their contrast in cellshot's palette (cellshot.Basic16)
	Floor      float64
}

// Floors16 are the contrasts Inks16 asks of a primary pin: 3:1 for the brand as text
// on the page, 4.5:1 for an ink on the brand's fill.
var Floors16 = map[string]float64{"primary": 3, "fgSelected": 4.5, "onPrimary": 4.5}

// Inks16 judges what a theme's ansi16 pin for primary draws, per mode, in the palette
// cellshot renders the 16 colours in (cellshot.Basic16, VS Code's terminal), the one
// the sheets show: primary as text on the page (bg), and on primary's fill the
// selection ink (fgSelected, skipped under focus "fg", where the selection is fg) and
// onPrimary (a filled tag, a bar header), each as the kit draws it (theme.Ink swaps an
// ink that lands on the fill's own slot). The truecolour checks judge the hex; a pin
// moves the brand to a slot they never saw: white on bright yellow is 1.17:1, yellow 3
// on white 1.35:1. A grey pin is skipped: the kit draws focus as fg there (WarmPin16).
func Inks16(f theme.File) []Ink16 {
	var out []Ink16
	for _, mode := range []string{"dark", "light"} {
		n, pinned := f.ANSI16[mode]["primary"]
		if !pinned || theme.Grey16(n) || f.Roles[mode] == (theme.Roles{}) {
			continue
		}
		dark := mode != "light"
		t := theme.Resolve16(f, dark)
		tc := theme.Resolve(f, dark, false).P
		p := t.P
		pairs := []struct {
			role, on  string
			ink, fill color.Color
			hex       string
		}{{"primary", "bg", p.Primary, p.Bg, hexOf(tc.Primary)}}
		if f.Expressions.Focus != "fg" {
			pairs = append(pairs, struct {
				role, on  string
				ink, fill color.Color
				hex       string
			}{"fgSelected", "primary", t.Ink(p.FgSelected, p.Primary), p.Primary, hexOf(tc.FgSelected)})
		}
		pairs = append(pairs, struct {
			role, on  string
			ink, fill color.Color
			hex       string
		}{"onPrimary", "primary", t.Ink(p.OnPrimary, p.Primary), p.Primary, hexOf(tc.OnPrimary)})
		for _, x := range pairs {
			a, b := theme.Slot16(x.ink), theme.Slot16(x.fill)
			if a < 0 || b < 0 {
				continue
			}
			r := oklch.Contrast(basic16Hex(a), basic16Hex(b))
			if r < Floors16[x.role] {
				out = append(out, Ink16{Mode: mode, Role: x.role, On: x.on, Hex: x.hex, Index: a, OnI: b, Ratio: r, Floor: Floors16[x.role]})
			}
		}
	}
	return out
}

// basic16Hex is ANSI colour n as cellshot draws it.
func basic16Hex(n int) string {
	c := cellshot.Basic16(n)
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}

// String is the finding for check's stderr, without the path.
func (x Ink16) String() string {
	fix := "pin primary to a slot it reads on, or drop the pin"
	switch x.Role {
	case "fgSelected", "onPrimary":
		fix = fmt.Sprintf("set %s to an ink that reads on %s too (a near-black on a light slot, a near-white on a dark one)", x.Role, slotName(x.OnI))
	}
	return fmt.Sprintf("%s: %s %s draws as ANSI %d on %s's ANSI %d at 16 colours (-profile ansi), %.2f:1 in cellshot's palette, under %.1f: %s",
		x.Mode, x.Role, x.Hex, x.Index, x.On, x.OnI, x.Ratio, x.Floor, fix)
}

// Clashes16 prints Findings16 on stderr, each with its pin, and returns how many fail.
func Clashes16(f theme.File, path string, stderr io.Writer) int {
	failed := 0
	for _, x := range Findings16(f) {
		pin := x.PinJSON()
		switch {
		case x.Fail && x.kind == clash16:
			failed++
			fmt.Fprintf(stderr, "palette: %s: %s; under focus \"fg\" primary is a state, so they read as one: pin %s with %s\n", path, x.What, x.Role, pin)
		case x.Fail:
			failed++
			fmt.Fprintf(stderr, "palette: %s: %s: pin it with %s\n", path, x.What, pin)
		case x.kind == clash16:
			fmt.Fprintf(stderr, "palette: warning: %s: %s, so those states differ there only by glyph and word; %s separates them\n", path, x.What, pin)
		case x.kind == share16:
			fmt.Fprintf(stderr, "palette: warning: %s: %s; %s separates them\n", path, x.What, pin)
		case x.kind == evict16:
			fmt.Fprintf(stderr, "palette: warning: %s: %s; %s moves primary off it\n", path, x.What, pin)
		default:
			fmt.Fprintf(stderr, "palette: warning: %s: %s; %s moves it\n", path, x.What, pin)
		}
	}
	return failed
}

// Pins16 are the ansi16 pins Findings16 recommends for f, applied until a 16-colour
// terminal reads it right (each round's pins are taken before the next looks again), on
// top of the pins f already has. nil when f needs none. palette derive writes them into
// its output, so its first check prints nothing.
func Pins16(f theme.File) map[string]map[string]int {
	pins := map[string]map[string]int{}
	for mode, m := range f.ANSI16 {
		pins[mode] = map[string]int{}
		for role, n := range m {
			pins[mode][role] = n
		}
	}
	added := false
	for range 4 {
		f.ANSI16 = pins
		found := Findings16(f)
		if len(found) == 0 {
			break
		}
		for _, x := range found {
			if pins[x.Mode] == nil {
				pins[x.Mode] = map[string]int{}
			}
			pins[x.Mode][x.Role] = x.Pin
			added = true
		}
	}
	if !added {
		return nil
	}
	return pins
}

// keepRank orders the state roles by how firmly the 16 colours own them: error is red
// everywhere, success green, warning yellow; primary and info are the brand's and
// move first. It decides which of a clashing pair moves, and the pin order when the
// brand yields a status role's own slot (PinOrder).
var keepRank = map[string]int{"error": 0, "success": 1, "warning": 2, "primary": 3, "info": 4}

// primaryFirst is the pin order when the brand is displaced too: after error, primary
// takes the free slot nearest its hue before the status roles take theirs, so an amber
// brand and its amber warning, both off red, leave the brand on yellow and move the
// warning, not the brand onto green or magenta.
var primaryFirst = map[string]int{"error": 0, "primary": 1, "success": 2, "warning": 3, "info": 4}

// ownHueMax is how near a status role's own slot the brand's hue may be before the
// brand is that colour and yields the slot (ownSlot): a yellow brand (#FFE600, 10° from
// yellow) yields it to warning in both modes; an amber (36° off) is not a yellow and
// takes it first. Less than half the 60° between ANSI's hues, with room to spare.
const ownHueMax = 25

// PinOrder is the order the state roles of mode take their pins in: primaryFirst when
// a slot within farPinMax of the brand's hue exists (an amber, 36° from yellow), since
// the brand drawn in a far hue beside that slot's own role reads as that role; keepRank
// when none does (an orange, 71° from yellow and from magenta alike: the status roles
// keep their own slots and the brand takes the nearest free), when the brand has no
// hue, or when the brand is a status role's own colour and yields its slot: natively on
// it in this mode (evicted), or within ownHueMax of its hue. A warm brand (WarmPin16)
// is primaryFirst: its slot is fixed, and the warning's follows it. Each mode decides
// for itself: a dark lime on 11 says nothing about a light amber primary.
func PinOrder(f theme.File, mode string, evicted bool) map[string]int {
	if evicted || f.Roles[mode] == (theme.Roles{}) {
		return keepRank
	}
	_, c, h := oklch.LCH(hexOf(theme.Resolve(f, mode != "light", false).P.Primary))
	if c < 0.04 {
		return keepRank
	}
	if warm(h) {
		return primaryFirst // its slot is set (WarmPin16), and the warning's yellow follows from it
	}
	for _, slots := range ownSlot {
		for _, n := range slots {
			if _, _, hs := oklch.LCH(hexOf(ansi.BasicColor(n))); hueDiff(h, hs) <= ownHueMax {
				return keepRank
			}
		}
	}
	for n := 2; n <= 6; n++ { // the hues that are not red: green, yellow, blue, magenta, cyan
		_, _, hs := oklch.LCH(hexOf(ansi.BasicColor(n)))
		if hueDiff(h, hs) <= farPinMax {
			return primaryFirst
		}
	}
	return keepRank
}

// FarPin is a pin that moves a role far from its own hue while a slot nearer it is
// free: the role reads as another state there (FarPins).
type FarPin struct {
	Mode, Role string
	Col        string  // the role as the theme writes it, truecolour hex
	Hue        float64 // its OKLCH hue
	Pin        int     // the pin it has
	Moved      float64 // degrees of hue between the two
	Nearer     int     // the pin the tool would give it (Pins16, with this pin and the later roles' lifted)
	Yielded    string  // the role whose pin held Nearer, "" when none
}

// farPinMax is how far a pin may move a role's hue before FarPins reports it: ANSI's
// six hues are 60° apart, so a move past that lands nearer another hue than the role's.
const farPinMax = 60

// FarPins are the theme's ansi16 pins that move a role with a hue (chroma 0.04 or
// more) more than farPinMax degrees of OKLCH hue from its truecolour value, when the
// tool itself would pin it nearer: Pins16 with that pin lifted, and with the pins of
// the roles after it in PinOrder lifted too, so a brand whose yellow is held by its
// warning's pin is told the yellow is its (derive gives it that way). A pin that is the
// nearest free hue (an orange brand on magenta, with red error's and yellow warning's)
// is not reported, however far: nothing nearer is free; nor is one the lifted theme
// pins nowhere (its native slot is right, or it yields a status role's own), since
// there is no nearer pin to name. Greys and neutrals carry no hue.
func FarPins(f theme.File) []FarPin {
	var out []FarPin
	if len(f.ANSI16) == 0 {
		return nil
	}
	unpinned := f
	unpinned.ANSI16 = nil
	native := Findings16(unpinned)
	for _, mode := range []string{"dark", "light"} {
		pins := f.ANSI16[mode]
		if len(pins) == 0 || f.Roles[mode] == (theme.Roles{}) {
			continue
		}
		evicted := slices.ContainsFunc(native, func(x Finding16) bool { return x.kind == evict16 && x.Mode == mode })
		order := PinOrder(f, mode, evicted)
		tc := theme.Resolve(f, mode != "light", false).P
		for _, role := range sortedRoles16(pins) {
			n := pins[role]
			if _, isState := order[role]; !isState || theme.Grey16(n) {
				continue
			}
			col := paletteHex(tc, role)
			_, c, h := oklch.LCH(col)
			if c < 0.04 {
				continue
			}
			if _, warm := WarmPin16(f, mode); warm && role == "primary" {
				continue // a warm brand off its slot is Findings16's (warm16), which names the slot
			}
			_, _, hs := oklch.LCH(hexOf(ansi.BasicColor(n)))
			moved := hueDiff(h, hs)
			if moved <= farPinMax {
				continue
			}
			// the tool's own pin for the role, with this pin and the later roles' lifted
			lifted := f
			lifted.ANSI16 = map[string]map[string]int{}
			for m, mp := range f.ANSI16 {
				lifted.ANSI16[m] = map[string]int{}
				for r, v := range mp {
					if m == mode && (r == role || order[r] > order[role]) {
						continue
					}
					lifted.ANSI16[m][r] = v
				}
			}
			nearer, ok := Pins16(lifted)[mode][role]
			if !ok || nearer == n || theme.Grey16(nearer) {
				continue // the tool pins it nowhere (its native slot is right, or yielded), or here: nothing nearer to name
			}
			if _, _, hn := oklch.LCH(hexOf(ansi.BasicColor(nearer))); hueDiff(h, hn) >= moved {
				continue // the tool's pin is no nearer its hue: nothing nearer to name
			}
			yielded := ""
			for _, r := range sortedRoles16(pins) {
				if r != role && pins[r] == nearer {
					yielded = r
				}
			}
			out = append(out, FarPin{Mode: mode, Role: role, Col: col, Hue: h, Pin: n, Moved: moved, Nearer: nearer, Yielded: yielded})
		}
	}
	return out
}

// warmMin is the lowest OKLCH hue warm calls warm: coral #D15D51 (28°) is warm, a
// crimson (20°) is a red, whose nearest free slot, magenta, is a red's pink.
const warmMin = 25

// yellowHue is ANSI yellow's OKLCH hue, 110°.
var yellowHue = func() float64 { _, _, h := oklch.LCH(hexOf(ansi.BasicColor(3))); return h }()

// warm reports an OKLCH hue between warmMin and yellow's own (ownHueMax short of ANSI
// yellow's 110°): the vermilions, oranges, coppers and ambers, 25°–85°. Their nearest
// slots are error's red, the warning's yellow and magenta.
func warm(h float64) bool {
	return h >= warmMin && h < yellowHue && hueDiff(h, yellowHue) > ownHueMax
}

// WarmPin16 is where the brand draws at 16 colours in mode when it has no slot of its
// own there, and whether it has none. A warm brand (warm: an orange, a copper, an
// amber) has none on either page: red is error's, yellow the warning's, and magenta the
// pink an orange's read forbids. On a dark page it takes bright yellow 11 and the warning
// keeps the other yellow, 3; the warning's glyph (▲ !), not its hue, tells them apart, so
// every warning mark carries one (ui.StateSparkline). On a light page it has no hue:
// yellow 3 is 1.35:1 on white and magenta is pink, so it pins to fg's grey (0), and the
// kit draws focus as fg and bold there (theme.ResolveProfile); its hue returns at 256
// colours and truecolour. A yellow brand (within ownHueMax of yellow) has none on a light
// page either: yellow is the warning's, and the slots it would move to, cyan 6 and green
// 2, are under 3:1 on white. A brand of chroma under 0.04 has no hue to place.
func WarmPin16(f theme.File, mode string) (int, bool) {
	if f.Roles[mode] == (theme.Roles{}) {
		return 0, false
	}
	_, c, h := oklch.LCH(hexOf(theme.Resolve(f, mode != "light", false).P.Primary))
	switch {
	case c < 0.04:
	case warm(h) && mode != "light":
		return 11, true
	case mode == "light" && (warm(h) || hueDiff(h, yellowHue) <= ownHueMax):
		return theme.GreySlot16("fg", false), true
	}
	return 0, false
}

// hueDiff is the smaller angle between two hues, in degrees.
func hueDiff(a, b float64) float64 {
	d := math.Abs(a - b)
	if d > 180 {
		d = 360 - d
	}
	return d
}

// slotName is a 16-colour slot's hue as a terminal names it.
func slotName(n int) string {
	names := []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"}
	if n >= 8 {
		return "bright " + names[n-8]
	}
	return names[n]
}

// farPinText is the warning for a far pin (FarPins).
func farPinText(x FarPin) string {
	_, _, hn := oklch.LCH(hexOf(ansi.BasicColor(x.Nearer)))
	hue := slotName(x.Pin)
	hue = strings.TrimPrefix(hue, "bright ")
	s := fmt.Sprintf("%s: %s %s is pinned to ANSI %d at 16 colours (-profile ansi), %s, %.0f° of hue from its own %.0f°, so %s reads as %s there; %s is nearer its hue (%s, %.0f° off)",
		x.Mode, x.Role, x.Col, x.Pin, slotName(x.Pin), x.Moved, x.Hue, roleText(x.Role), hue, Finding16{Mode: x.Mode, Role: x.Role, Pin: x.Nearer}.PinJSON(), strings.TrimPrefix(slotName(x.Nearer), "bright "), hueDiff(x.Hue, hn))
	if x.Yielded != "" {
		s += fmt.Sprintf(", with %s moved off it", x.Yielded)
	}
	return s
}

// roleText is what a state role is on screen, for a finding's sentence.
func roleText(role string) string {
	if role == "primary" {
		return "the brand"
	}
	return "a " + role
}

// paletteHex is a state role of a resolved palette as hex.
func paletteHex(p theme.Palette, role string) string {
	switch role {
	case "primary":
		return hexOf(p.Primary)
	case "error":
		return hexOf(p.Error)
	case "warning":
		return hexOf(p.Warning)
	case "success":
		return hexOf(p.Success)
	case "info":
		return hexOf(p.Info)
	case "secondary":
		return hexOf(p.Secondary)
	case "tertiary":
		return hexOf(p.Tertiary)
	case "accent":
		return hexOf(p.Accent)
	}
	return ""
}

// checkExpressions fails an expression key the kit doesn't read, then a value it
// doesn't know (theme.CheckExpressions), each with what it does know.
func checkExpressions(raw json.RawMessage) error {
	keys, err := pyfmt.Keys(raw)
	if err != nil {
		return fmt.Errorf("expressions: %w", err)
	}
	for _, k := range keys {
		if !theme.ExpressionKey(k) {
			return fmt.Errorf("expressions.%s is not a key the kit reads (%s): a typo, or a key from a newer kit, is drawn as if unset", k, strings.Join(exprKeys(), ", "))
		}
	}
	var x theme.Expressions
	if err := json.Unmarshal(raw, &x); err != nil {
		return fmt.Errorf("expressions: %w", err)
	}
	return theme.CheckExpressions(x)
}

// exprKeys are the expression keys the kit reads, in the contract's order.
func exprKeys() []string {
	t := reflect.TypeOf(theme.Expressions{})
	out := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		k, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		out = append(out, k)
	}
	return out
}

// roleKeys are the role names the engine reads (theme.Roles' JSON keys). check
// validates each of them, not only the ones a contrast check reads, so it fails
// whatever theme.Load fails.
var roleKeys = func() map[string]bool {
	m := map[string]bool{}
	t := reflect.TypeOf(theme.Roles{})
	for i := range t.NumField() {
		k, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		m[k] = true
	}
	return m
}()

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func verdict(bad int) string {
	if bad > 0 {
		return fmt.Sprintf("FAIL (%d contrast problems)", bad)
	}
	return "OK (0 contrast problems)"
}

func derive(args []string, stdout, stderr io.Writer) int {
	var pos []string
	opts := map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			pos = append(pos, a)
			continue
		}
		if k, v, ok := strings.Cut(a[2:], "="); ok {
			opts[k] = v
			continue
		}
		if i+1 >= len(args) {
			return fail(stderr, fmt.Errorf("%s needs a value", a))
		}
		opts[a[2:]] = args[i+1]
		i++
	}
	if len(pos) == 0 {
		fmt.Fprintln(stderr, `usage: palette derive "#primary" ["#secondary"] [--accent ..] [--bg ..] [--light-bg ..] [--tint 0.012] [--name ..] [--out ..] [--swatch ..]`)
		return 2
	}
	secondary := ""
	if len(pos) > 1 {
		secondary = pos[1]
	}
	in := []struct{ what, hex string }{
		{"primary", pos[0]}, {"secondary", secondary}, {"--accent", opts["accent"]},
		{"--bg", opts["bg"]}, {"--light-bg", opts["light-bg"]},
	}
	for i, c := range in {
		if (i == 0 || c.hex != "") && !oklch.Valid(c.hex) {
			return fail(stderr, fmt.Errorf("%s: %q is not a hex colour", c.what, c.hex))
		}
	}
	tint := 0.012
	if s, ok := opts["tint"]; ok {
		t, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return fail(stderr, fmt.Errorf("--tint: %q is not a number", s))
		}
		tint = t
	}
	name := "brand"
	if s, ok := opts["name"]; ok {
		name = s
	}
	modes := Derive(pos[0], secondary, opts["accent"], opts["bg"], opts["light-bg"], tint)
	pins := Pins16(themeOf(modes))
	js := JSON(name, modes, pins)
	if out, ok := opts["out"]; ok {
		if err := os.WriteFile(out, []byte(js+"\n"), 0o644); err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintf(stdout, "wrote %s\n", out)
	} else {
		fmt.Fprintln(stdout, js)
	}
	bad := 0
	for _, m := range modes {
		b, txt := Report(m.Roles, m.Name, nil)
		bad += b
		fmt.Fprintln(stderr, txt)
		for _, w := range Warnings(m.Roles, m.Name) {
			fmt.Fprintf(stderr, "warning: %s\n", w)
		}
		if d, near := NearPrimary(m.Roles); near {
			fmt.Fprintf(stderr, "warning: %s\n", nearPrimaryText(m.Name, m.Roles, d))
		}
	}
	if len(pins) > 0 {
		var parts []string
		for _, mode := range []string{"dark", "light"} {
			for _, role := range sortedRoles16(pins[mode]) {
				parts = append(parts, fmt.Sprintf("%s %s %d", mode, role, pins[mode][role]))
			}
		}
		fmt.Fprintf(stderr, "pinned at 16 colours (\"ansi16\", kept in the theme): %s\n", strings.Join(parts, ", "))
		tf := themeOf(modes)
		tf.ANSI16 = pins
		for _, x := range FarPins(tf) {
			fmt.Fprintf(stderr, "warning: %s\n", farPinText(x))
		}
		for _, x := range Inks16(tf) {
			fmt.Fprintf(stderr, "warning: %s\n", x)
		}
	}
	if sw, ok := opts["swatch"]; ok {
		if err := os.WriteFile(sw, []byte(Swatch(modes)), 0o644); err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintf(stderr, "wrote %s (render: cellshot %s swatch.png)\n", sw, sw)
	}
	fmt.Fprintln(stderr, verdict(bad))
	if bad > 0 {
		return 1
	}
	return 0
}

// themeOf is the derive document as the engine reads it, for the 16-colour pass.
func themeOf(modes []Mode) theme.File {
	f := theme.File{Roles: map[string]theme.Roles{}}
	for _, m := range modes {
		b, _ := json.Marshal(m.Roles)
		var r theme.Roles
		_ = json.Unmarshal(b, &r)
		f.Roles[m.Name] = r
	}
	return f
}

func sortedRoles16(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// nearPrimaryText is the warning for a warning role beside primary (NearPrimary).
func nearPrimaryText(mode string, r map[string]string, d float64) string {
	return fmt.Sprintf("%s: warning %s sits beside primary %s in hue and chroma (%.3f apart in OKLab a/b, under %.2f), so a warning row reads as a focused one; move warning's hue, or pin a 16-colour slot and accept it in the read",
		mode, r["warning"], r["primary"], d, nearPrimaryMax)
}
