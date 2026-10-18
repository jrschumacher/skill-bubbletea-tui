// Package theme turns a brand theme file (theme.json) into the styles every
// screen uses. Screens never hold a hex value or a glyph literal: they ask the
// theme. A brand changes the JSON; the screens stay the same.
//
// Three layers, all in the JSON:
//
//	roles        the palette by role name, a dark and a light set
//	glyphs       the brand's glyph vocabulary (rule, focus bar, cursor, status…)
//	expressions  how each idiom element is drawn (header, selection, buttons…)
//
// Missing roles are derived (see derive.go), missing glyphs and expressions
// fall back to defaults, so a partial theme still renders.
package theme

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

// Roles is the palette by role. Hex strings in the JSON.
type Roles struct {
	Primary   string `json:"primary"`   // the brand hue: focus, current, selected rows, header rule (under expressions.focus "fg": only what the theme or app asks for)
	Secondary string `json:"secondary"` // second hue: primary buttons, gradient end, subtitles
	Tertiary  string `json:"tertiary"`  // a third voice: inline code, links; prompts and carets where primary doesn't read on bgOverlay
	Accent    string `json:"accent"`    // section titles, headings; reads as "heading", never "alert"

	Bg        string `json:"bg"`        // the background the brand expects
	BgPanel   string `json:"bgPanel"`   // one step up: recessed/raised panels (tool output, cards)
	BgSubtle  string `json:"bgSubtle"`  // two steps: unselected buttons, inactive chips, dim bars
	BgOverlay string `json:"bgOverlay"` // three steps: popovers, completions, hover

	Fg         string `json:"fg"`         // body text
	FgSoft     string `json:"fgSoft"`     // long-form prose (slightly softer than fg)
	FgMuted    string `json:"fgMuted"`    // secondary text: keys, done steps, metadata
	FgSubtle   string `json:"fgSubtle"`   // tertiary text: hints, placeholders (never the only carrier of meaning)
	FgSelected string `json:"fgSelected"` // text on a primary-filled selected row

	Border      string `json:"border"`      // rules: barely lighter than bg
	BorderFocus string `json:"borderFocus"` // the one dialog's border

	Success string `json:"success"`
	Error   string `json:"error"`
	Warning string `json:"warning"`
	Info    string `json:"info"`

	OnPrimary   string `json:"onPrimary"`   // text on primary fill
	OnSecondary string `json:"onSecondary"` // text on secondary fill
	OnError     string `json:"onError"`
	OnWarning   string `json:"onWarning"`
	OnSuccess   string `json:"onSuccess"`

	White string `json:"white,omitempty"` // legacy themes: used as OnPrimary/OnSecondary when those are missing
}

// Glyphs is the brand's glyph vocabulary. Every glyph has an ASCII twin (see ASCII)
// drawn when the theme is resolved with ascii=true: from the theme's own ascii map,
// else the kit's. CheckGlyphs (palette check, the kit's tests) fails a glyph with
// neither, and two marks of one meaning set that share a twin. Load doesn't: at run
// time an unmapped glyph draws as "?", which tuilint frames --ascii fails. A project
// that adds a glyph field here gets it mapped and checked too.
type Glyphs struct {
	HeaderRule string   `json:"headerRule"` // fills the header after the wordmark: ╱ ─ ═ · ━ ╌ ~ (or "")
	Rule       string   `json:"rule"`       // section rules: ─ ━ ╌ ┄ ═ ·
	FocusBar   string   `json:"focusBar"`   // left marker of the focused block: ▌ ┃ │ ▎ ❯
	BlurBar    string   `json:"blurBar"`    // the same position, unfocused: ╎ ┆ ┊, ASCII : ("" = ╎ beside a blurred selection, Theme.BlurBar; padding only in a stream or form); never the focus bar
	Cursor     string   `json:"cursor"`     // selection cursor in lists/selects: › ❯ > ▸ →
	Prompt     string   `json:"prompt"`     // input prompt: > $ ❯ ›
	Done       string   `json:"done"`       // ✓ ✔ ■ ◆ x
	Current    string   `json:"current"`    // ● ◆ ▶ >
	Todo       string   `json:"todo"`       // ○ ◇ · □ -
	Fail       string   `json:"fail"`       // × ✗ ✕ !
	Warn       string   `json:"warn"`       // ▲ ! ◬ (status: degraded) — must differ in shape from Current and Fail
	Pending    string   `json:"pending"`    // ● ◌ ·
	Partial    string   `json:"partial"`    // ◐ ◑ ◒ (in progress: between current and done; "reading", "rolling", a running tool)
	Bullet     string   `json:"bullet"`     // • · ▪ -
	Sep        string   `json:"sep"`        // separator in detail lines: · • │ /
	Arrow      string   `json:"arrow"`      // → › ▸
	Ellipsis   string   `json:"ellipsis"`   // …
	RadioOn    string   `json:"radioOn"`    // ◉ ● (•)
	RadioOff   string   `json:"radioOff"`   // ○ ◯ ( )
	CheckOn    string   `json:"checkOn"`    // ☑ [x] ■
	CheckOff   string   `json:"checkOff"`   // ☐ [ ] □
	TreeMid    string   `json:"treeMid"`    // ├──
	TreeEnd    string   `json:"treeEnd"`    // ╰── └──
	TreeRail   string   `json:"treeRail"`   // │
	Thumb      string   `json:"thumb"`      // scrollbar thumb ┃ █
	Track      string   `json:"track"`      // scrollbar track │ ░ (or " ")
	Spinner    []string `json:"spinner"`    // frames
	FieldA     string   `json:"fieldA"`     // backdrop field bright glyph ◆ ▲ ╱
	FieldB     string   `json:"fieldB"`     // backdrop field subtle glyph ◇ △

	Caret         string `json:"caret"`         // input caret after the typed text: ▏ _ (ASCII '; never a bar glyph or its twin |, the focus bar's: CheckGlyphs fails it)
	More          string `json:"more"`          // marks lines scrolled off ("↑ 12 more"): ↑ ▲ ^
	UpDown        string `json:"upDown"`        // the arrow-key pair as footers name it: ↑↓ (ascii ^v)
	LeftRight     string `json:"leftRight"`     // ←→ (ascii <>)
	Back          string `json:"back"`          // a form's back link: ← ‹ <
	ProgressFill  string `json:"progressFill"`  // the filled part of a progress bar: ━ █ ■
	ProgressEmpty string `json:"progressEmpty"` // the rest of it: ╌ ░ ·
	ProgressHead  string `json:"progressHead"`  // the half cell at the fill's end: ╸ ▌
	TurnBar       string `json:"turnBar"`       // the user's turn in a chat when blurred: │ ┆ (focusBar when focused; blurBar wins when set)
	TabUnderline  string `json:"tabUnderline"`  // the active tab's underline under tabs "underline": ▔ ─ ▀ (ASCII draws the rule glyph instead)

	// ASCII is the theme's own twins, glyph → ASCII, for glyphs the kit's map lacks
	// or draws differently: {"✎": ">", "◔": "~"}. It wins over the kit's map.
	ASCII map[string]string `json:"ascii,omitempty"`
}

// Expressions are the brand's choices for how each element is drawn. These are
// the axes that make two themes read as two products, not one app recoloured.
type Expressions struct {
	Header       string `json:"header"`       // rule | bar | minimal | stamp | banner
	Wordmark     string `json:"wordmark"`     // gradient | solid | inverse | tracked | bracket | upper | plain (fg, bold, no hue)
	SectionTitle string `json:"sectionTitle"` // accent | primary | muted-upper | fg
	TitleCase    string `json:"titleCase"`    // sentence | upper | lower
	LabelCase    string `json:"labelCase"`    // upper | lower | as-is: table headers, tags, toast and summary labels, and the kit's fixed labels (tab and step names, any number of words; its sentences such as "Canceled." take titleCase; unset: headers upper, the fixed copy in titleCase, the rest as the kit draws them)
	Selection    string `json:"selection"`    // bar | fill | caret | inverse  (how a selected/focused row is marked)
	Button       string `json:"button"`       // pill | bracket | underline | inverse
	Tag          string `json:"tag"`          // filled | bracket | bold
	Dialog       string `json:"dialog"`       // rounded | square | double | heavy | none (filled panel) | rules (filled panel, rule above and below, title in the top rule)
	DialogTitle  string `json:"dialogTitle"`  // rule | plain | bar   (rule = title + headerRule glyph run in a gradient; unused under dialog rules)
	Tabs         string `json:"tabs"`         // underline | pill | bracket | slash
	Footer       string `json:"footer"`       // keys | bar | rule
	Keys         string `json:"keys"`         // inline | boxed
	Density      string `json:"density"`      // airy | compact
	Backdrop     string `json:"backdrop"`     // none | field
	Working      string `json:"working"`      // spinner | scramble   (the hero "working" indicator; scramble is Crush's)
	Spinner      string `json:"spinner"`      // focus | primary | secondary | tertiary | accent | info | fg | muted: the ink of the working marks (spinner frames, a busy row's mark, a running tool); focus = primary, or fg under focus "fg"
	Scrim        string `json:"scrim"`        // dim | none   (dim the screen under a dialog)
	// Focus is the ink of the kit's own emphasis: primary (default), or fg, where the
	// focus bar, cursor, prompt, current step, active tab, selection and focused button
	// are fg and bold, and the kit's marks (spinner, progress, busy dot, header and
	// dialog-title rules, the dialog border unless borderFocus is set) are neutral.
	// For a brand whose hue is reserved for a state. Fills the theme opts into (a bar
	// header, a bar dialog title, a filled tag, an inverse wordmark) keep primary.
	Focus   string `json:"focus"`   // primary | fg
	PaintBg bool   `json:"paintBg"` // own the terminal background (tea.View.BackgroundColor)
	Gutter  int    `json:"gutter"`  // side margin in cells (default 2)
	Measure int    `json:"measure"` // max prose width (default 100)
}

// The three tones of a shade wordmark (scripts/wordmark.py --style shade). The kit tones
// them full → accent, dark → primary, mid → muted; they are the script's glyphs, not the
// brand's, so they are constants here rather than Glyphs.
const (
	ShadeFull = "█"
	ShadeDark = "▓"
	ShadeMid  = "▒"
)

// Ellipsis is the kit's default ellipsis, for a cut made where no theme is at hand
// (ui.Fit); ui.Trunc cuts with the theme's own glyphs.ellipsis.
const Ellipsis = "…"

// SparkRamp is the eight heights of a sparkline, lowest first, and SparkRampASCII what
// an ASCII terminal draws in their place (controls.go Sparkline): no "*" or "+", the
// current and done twins, so a peak never reads as a status mark beside the dots.
const (
	SparkRamp      = "▁▂▃▄▅▆▇█"
	SparkRampASCII = "_.-=:;%#@"
)

// Wordmark is an optional multi-row wordmark (scripts/wordmark.py output).
type Wordmark struct {
	Rows  []string `json:"rows"`
	Style string   `json:"style"` // half | shade | ascii | tracked: the style wordmark.py drew the rows with. Metadata: the kit reads the rows, whose glyphs (█ ▓ ▒) carry the tones
}

// LadderRung is one rung of the product's own status ladder (File.Ladder): a glyph,
// what it means to the product, and the role it draws in. ui.Mark(t, meaning) draws it,
// ui.Rungs lists it and the showcase's ladder state shows it, so a sheet can judge the
// product's states and not only the kit's slots.
type LadderRung struct {
	Glyph   string `json:"glyph"`   // a status slot (StatusSlots: that glyph, and its twin under ASCII) or a one-cell literal with a twin in glyphs.ascii or the kit's map
	Meaning string `json:"meaning"` // the name the product gives the state (new, reading, reviewed): what ui.Mark takes; never a kit slot's name (KitSlots), whose ink is the kit's
	Role    string `json:"role"`    // the ink: any colour role (primary, fgSubtle, success, …), or focus / working, the inks expressions.focus and expressions.spinner resolve to
}

// File is the theme JSON.
type File struct {
	Name        string           `json:"name"`
	Brand       string           `json:"brand"` // the CLI's own name; flows into headers, paths, env vars
	Read        string           `json:"read"`  // one paragraph: why these choices (see references/brand.md)
	Roles       map[string]Roles `json:"roles"` // "dark", "light"
	Glyphs      Glyphs           `json:"glyphs"`
	Expressions Expressions      `json:"expressions"`
	Wordmark    Wordmark         `json:"wordmark"`
	// ANSI16 pins roles to one of the 16 colours (0-15) per mode, used only when the
	// terminal has 16 (ResolveProfile with colorprofile.ANSI): {"dark": {"primary": 11}}.
	// Downsampling picks the nearest of 16 by distance, which lands an amber on red, the
	// error's slot; no hex fixes that without leaving amber at 256 colours. A role derived
	// from a pinned one (secondary from primary) follows it. See ANSI16Clashes.
	ANSI16 map[string]map[string]int `json:"ansi16,omitempty"`
	// Ladder is the product's own status ladder, in the order a thing moves through it:
	// [{"glyph": "current", "meaning": "new", "role": "primary"}, …]. Set, ui.Rungs and
	// the ladder state draw these rungs instead of the kit's default ones, and
	// ui.Mark(t, "new") draws the rung so named; the kit's slots (done, current, …) keep
	// their own inks. See CheckLadder (palette check fails a role or glyph the kit
	// doesn't know); Load keeps loading.
	Ladder []LadderRung `json:"ladder,omitempty"`
}

// Load reads a theme file: Parse on its bytes, errors named by the path.
func Load(path string) (File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	f, err := Parse(b)
	if err != nil {
		return f, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// Parse reads a theme from its JSON, as Load does from a file: for a theme the binary
// carries (go:embed, see themes/embed.go) or one fetched at run time. Every role colour
// must be "#RGB" or "#RRGGBB" (see CheckHex), or empty to be derived: Lip Gloss draws
// anything else as a different colour or none, so a theme that would render wrong fails
// here instead.
func Parse(b []byte) (File, error) {
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return f, err
	}
	modes := make([]string, 0, len(f.Roles))
	for m := range f.Roles {
		modes = append(modes, m)
	}
	sort.Strings(modes)
	for _, m := range modes {
		if err := f.Roles[m].check(); err != nil {
			return f, fmt.Errorf("roles.%s.%w", m, err)
		}
	}
	if err := CheckANSI16(f); err != nil {
		return f, err
	}
	return f, nil
}

// CheckANSI16 validates the theme's ansi16 pins: a mode (dark, light), a role the
// theme contract knows, and an index 0-15. They are colours, so Load fails on them as
// on a bad hex: a pin that names no role or no colour would silently draw nothing.
func CheckANSI16(f File) error {
	for _, m := range sortedKeys(f.ANSI16) {
		if m != "dark" && m != "light" {
			return fmt.Errorf("ansi16.%s: not a mode (dark, light)", m)
		}
		for _, role := range sortedKeys(f.ANSI16[m]) {
			if !roleKey(role) {
				return fmt.Errorf("ansi16.%s.%s: not a role (primary, error, warning, success, info, …)", m, role)
			}
			if n := f.ANSI16[m][role]; n < 0 || n > 15 {
				return fmt.Errorf("ansi16.%s.%s: %d is not one of the 16 colours (0-15)", m, role, n)
			}
		}
	}
	return nil
}

// wordmarkStyles are the styles scripts/wordmark.py writes into wordmark.style.
var wordmarkStyles = []string{"half", "shade", "ascii", "tracked"}

// CheckKeys reports a key under roles.<mode>, glyphs or wordmark that the kit doesn't
// read ("primay", "focusbar"), and a wordmark.style it doesn't know, each with what it
// does know. Load ignores an unknown key, so the role it meant falls back to the derived
// colour and nothing says why; palette check fails on it, as on an expressions key. Keys
// are matched exactly: encoding/json would read "focusbar" as focusBar, but the contract
// is written one way. data is the theme JSON, or a bare roles object ({"dark": …}). Any
// other error in the JSON is left to Load.
func CheckKeys(data []byte) error {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil
	}
	exact := func(raw json.RawMessage, where string, known []string) error {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil
		}
		for _, k := range sortedKeys(obj) {
			if !slices.Contains(known, k) {
				return fmt.Errorf("%s.%s is not a key the kit reads (%s): a typo, or a key from a newer kit, is drawn as if unset", where, k, strings.Join(known, ", "))
			}
		}
		return nil
	}
	modes := map[string]json.RawMessage{}
	if raw, ok := top["roles"]; ok {
		_ = json.Unmarshal(raw, &modes)
	} else {
		for _, m := range []string{"dark", "light"} {
			if raw, ok := top[m]; ok {
				modes[m] = raw
			}
		}
	}
	for _, m := range sortedKeys(modes) {
		if err := exact(modes[m], "roles."+m, jsonKeys(reflect.TypeOf(Roles{}))); err != nil {
			return err
		}
	}
	if raw, ok := top["glyphs"]; ok {
		if err := exact(raw, "glyphs", jsonKeys(reflect.TypeOf(Glyphs{}))); err != nil {
			return err
		}
	}
	if raw, ok := top["wordmark"]; ok {
		if err := exact(raw, "wordmark", jsonKeys(reflect.TypeOf(Wordmark{}))); err != nil {
			return err
		}
		var w Wordmark
		if err := json.Unmarshal(raw, &w); err == nil && w.Style != "" && !slices.Contains(wordmarkStyles, w.Style) {
			return fmt.Errorf("wordmark.style %q is not a style wordmark.py writes: %s", w.Style, strings.Join(wordmarkStyles, " | "))
		}
	}
	if raw, ok := top["ladder"]; ok {
		var rungs []json.RawMessage
		if err := json.Unmarshal(raw, &rungs); err != nil {
			return fmt.Errorf("ladder: not an array of rungs [{\"glyph\": …, \"meaning\": …, \"role\": …}]")
		}
		for i, r := range rungs {
			if err := exact(r, fmt.Sprintf("ladder[%d]", i), jsonKeys(reflect.TypeOf(LadderRung{}))); err != nil {
				return err
			}
		}
		var f File
		if err := json.Unmarshal(data, &f); err == nil {
			return CheckLadder(f)
		}
	}
	return nil
}

// ladderInks are the two inks a rung may name beside the colour roles: what
// expressions.focus and expressions.spinner resolve to (ui.RoleInk).
var ladderInks = []string{"focus", "working"}

// StatusSlots are the glyph keys a ladder rung may name instead of a literal: the status
// marks, each with its twin. Not the chrome glyphs (focusBar, caret), whose twins are
// bars a rung must never be.
var StatusSlots = []string{"done", "current", "todo", "pending", "partial", "warn", "fail"}

// KitSlots are the names the kit's own components draw by (ui.Mark, ui.Dot): a rung's
// meaning can't be one, or the rung would recolour the kit's slot.
var KitSlots = []string{"done", "current", "todo", "pending", "partial", "busy", "warn", "fail"}

// CheckLadder reports a rung of the theme's ladder the kit can't draw: a role that is
// neither a colour role nor focus / working, a meaning that is a kit slot's name (the
// kit's slots keep their inks), a glyph that is neither a status slot nor a one-cell
// literal with an ASCII twin, two rungs with one meaning, or two rungs that draw as
// one mark under ASCII. palette check fails on it (through CheckKeys); Load doesn't,
// so an app still starts, drawing the rung in fg.
func CheckLadder(f File) error {
	g := ASCII(glyphDefaults(f.Glyphs))
	seen, twins := map[string]int{}, map[string]int{}
	for i, r := range f.Ladder {
		where := fmt.Sprintf("ladder[%d]", i)
		if r.Meaning == "" {
			return fmt.Errorf("%s.meaning is empty: name the state (new, reading, reviewed)", where)
		}
		if slices.Contains(KitSlots, r.Meaning) {
			return fmt.Errorf("%s.meaning %q is a kit slot, whose ink is the kit's: name the product's state (new, reading, reviewed)", where, r.Meaning)
		}
		if j, dup := seen[r.Meaning]; dup {
			return fmt.Errorf("%s.meaning %q is also ladder[%d]'s: one name per state", where, r.Meaning, j)
		}
		seen[r.Meaning] = i
		if !roleKey(r.Role) && !slices.Contains(ladderInks, r.Role) {
			return fmt.Errorf("%s.role %q is not a colour role (primary, fgSubtle, success, …) or focus / working", where, r.Role)
		}
		tw, ok := GlyphSlot(g, r.Glyph)
		if !ok {
			tw = twin(f.Glyphs.ASCII, r.Glyph)
		}
		if r.Glyph == "" || tw == "?" || !ok && (lipgloss.Width(r.Glyph) != 1 || lipgloss.Width(tw) != 1) {
			return fmt.Errorf("%s.glyph %q is neither a status slot (%s) nor a one-cell glyph with a one-cell ASCII twin: add \"ascii\": {%q: \"<twin>\"} to glyphs", where, r.Glyph, strings.Join(StatusSlots, ", "), r.Glyph)
		}
		if j, dup := twins[tw]; dup {
			return fmt.Errorf("%s.glyph %q and ladder[%d].glyph %q both draw as %q under ASCII, so two states read as one", where, r.Glyph, j, f.Ladder[j].Glyph, tw)
		}
		twins[tw] = i
	}
	return nil
}

// GlyphSlot is the status glyph in g with JSON key key (StatusSlots: done, current, …)
// and whether key names one. A ladder rung's glyph may be a slot's name instead of a
// literal; any other string, a chrome glyph's key included, is a literal.
func GlyphSlot(g Glyphs, key string) (string, bool) {
	if !slices.Contains(StatusSlots, key) {
		return "", false
	}
	v, t := reflect.ValueOf(g), reflect.TypeOf(g)
	for i := range t.NumField() {
		if k, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ","); k == key {
			return v.Field(i).String(), true
		}
	}
	return "", false
}

// jsonKeys are t's fields' JSON keys, in the contract's order.
func jsonKeys(t reflect.Type) []string {
	out := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		if k, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ","); k != "" {
			out = append(out, k)
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// roleKey reports whether key is a role's JSON key (white, a legacy fallback, isn't one
// a theme pins).
func roleKey(key string) bool {
	t := reflect.TypeOf(Roles{})
	for i := range t.NumField() {
		if k, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ","); k == key && k != "white" {
			return true
		}
	}
	return false
}

// paletteRole is the resolved colour of the role with JSON key key (nil for none).
func paletteRole(p Palette, key string) color.Color {
	v := reflect.ValueOf(p)
	f := v.FieldByName(strings.ToUpper(key[:1]) + key[1:])
	if !f.IsValid() || f.IsNil() {
		return nil
	}
	return f.Interface().(color.Color)
}

// StateRoles are the roles that mark a state: ok, error, warning, info, and primary
// under focus "fg", where the brand hue is a state and nothing else (under focus
// "primary" it is the kit's emphasis, which may sit near red by design). Two of them on
// one colour make two states read as one.
var StateRoles = []string{"primary", "error", "warning", "success", "info"}

// stateRoles are StateRoles as f uses them.
func stateRoles(f File) []string {
	if f.Expressions.Focus == "fg" {
		return StateRoles
	}
	return StateRoles[1:]
}

// Clash is two state roles that differ in the theme but draw as one of the 16 colours.
type Clash struct {
	Mode       string      // dark | light
	A, B       string      // role keys, in StateRoles order
	ColA, ColB color.Color // the two as the theme writes them (truecolour)
	Index      int         // the ANSI colour both land on
}

func (c Clash) String() string {
	return fmt.Sprintf("%s: %s and %s both ANSI %d", c.Mode, c.A, c.B, c.Index)
}

// ANSI16Clashes are the pairs of state roles (StateRoles, primary only under focus "fg")
// that differ at truecolour but land on one of the 16 colours after the theme's ansi16
// pins, per mode the theme defines. Roles the theme makes identical (info left to
// default to primary) are one colour on purpose and aren't reported. palette check
// warns on each, and fails one with primary; the fix is an ansi16 pin for one of the two.
func ANSI16Clashes(f File) []Clash {
	var out []Clash
	for _, mode := range modes16(f) {
		dark := mode != "light"
		tc := ResolveProfile(f, dark, false, colorprofile.TrueColor).P
		an := ResolveProfile(f, dark, false, colorprofile.ANSI).P
		roles := stateRoles(f)
		for i, a := range roles {
			for _, b := range roles[i+1:] {
				if sameColor(paletteRole(tc, a), paletteRole(tc, b)) {
					continue
				}
				ia, oka := colorprofile.ANSI.Convert(paletteRole(an, a)).(ansi.BasicColor)
				ib, okb := colorprofile.ANSI.Convert(paletteRole(an, b)).(ansi.BasicColor)
				if oka && okb && ia == ib {
					out = append(out, Clash{mode, a, b, paletteRole(tc, a), paletteRole(tc, b), int(ia)})
				}
			}
		}
	}
	return out
}

// RedSlot is a state role misplaced around red at 16 colours: error off red, or another
// state on it.
type RedSlot struct {
	Mode  string      // dark | light
	Role  string      // role key
	Col   color.Color // as the theme writes it (truecolour)
	Index int         // the ANSI colour it lands on
}

func (r RedSlot) String() string {
	if r.Role == "error" {
		return fmt.Sprintf("%s: error draws as ANSI %d, not red (1 or 9)", r.Mode, r.Index)
	}
	return fmt.Sprintf("%s: %s draws as ANSI %d, error's red", r.Mode, r.Role, r.Index)
}

// ANSI16Red reports, per mode, the state roles a 16-colour terminal would read wrong:
// error landing anywhere but red (1 or 9), where a grey error beside a red warning
// reverses the two, and any other state role landing on red, error's slot. primary is
// reported under every focus: under "fg" it is a state, under "primary" the focus colour,
// drawn in error's red. A role the theme makes identical to error is skipped. palette
// check fails error off red and primary on red under focus "fg", and warns on the rest.
func ANSI16Red(f File) []RedSlot {
	var out []RedSlot
	red := func(i int) bool { return i == 1 || i == 9 }
	for _, mode := range modes16(f) {
		tc := ResolveProfile(f, mode != "light", false, colorprofile.TrueColor).P
		for _, role := range StateRoles {
			i := ANSI16Index(f, mode, role)
			switch {
			case i < 0:
			case role == "error" && !red(i):
				out = append(out, RedSlot{mode, role, tc.Error, i})
			case role != "error" && red(i) && !sameColor(paletteRole(tc, role), tc.Error):
				out = append(out, RedSlot{mode, role, paletteRole(tc, role), i})
			}
		}
	}
	return out
}

// modes16 are the theme's modes a 16-colour terminal resolves (dark, light), sorted.
func modes16(f File) []string {
	var out []string
	for _, m := range sortedKeys(f.Roles) {
		if m == "dark" || m == "light" {
			out = append(out, m)
		}
	}
	return out
}

// ANSI16Index is the 16-colour index role draws as in mode (dark, light), after the
// theme's ansi16 pins: what a terminal with 16 colours shows. -1 for no such role.
func ANSI16Index(f File, mode, role string) int {
	return Slot16(paletteRole(ResolveProfile(f, mode != "light", false, colorprofile.ANSI).P, role))
}

// Slot16 is the one of the 16 colours c draws as on a 16-colour terminal: c itself when
// it is one (a pinned role), else the nearest. -1 for nil.
func Slot16(c color.Color) int {
	if c == nil {
		return -1
	}
	if b, ok := colorprofile.ANSI.Convert(c).(ansi.BasicColor); ok {
		return int(b)
	}
	return -1
}

// NeutralRoles are the roles that carry no state: the page, its steps, its rules, the
// dialog's edge and the text ramp. At 16 colours they belong on the four greys (0, 7, 8,
// 15): on a hue, body text reads as a state and the page as a fill; on red, hints read
// as errors and a ruled dialog's edge as an error's. Every role a dialog draws is here
// (border, borderFocus, bgPanel, bgOverlay, fgSoft for its text), so one notch of
// lightness in a tinted borderFocus never decides between grey and red.
var NeutralRoles = []string{"bg", "bgPanel", "bgSubtle", "bgOverlay", "border", "borderFocus", "fg", "fgSoft", "fgMuted", "fgSubtle"}

// TextRamp are the text levels that stay apart at 16 colours: three on a dark page, two
// on a light one, where fgSubtle shares fgMuted's grey by design (GreySlot16). fgSoft is
// prose and may share fg's grey.
var TextRamp = []string{"fg", "fgMuted", "fgSubtle"}

// NeutralSlot is a neutral role misplaced at 16 colours: on a hue, or sharing a grey with
// another level of the text ramp.
type NeutralSlot struct {
	Mode  string      // dark | light
	Role  string      // role key
	Col   color.Color // as the theme writes it (truecolour)
	Index int         // the ANSI colour it lands on
	With  string      // the text ramp role it shares Index with; "" when Index is a hue
}

func (n NeutralSlot) String() string {
	if n.With != "" {
		return fmt.Sprintf("%s: %s and %s both ANSI %d", n.Mode, n.With, n.Role, n.Index)
	}
	return fmt.Sprintf("%s: %s draws as ANSI %d, a hue", n.Mode, n.Role, n.Index)
}

// Grey16 reports whether i is one of the four greys at 16 colours.
func Grey16(i int) bool { return i == 0 || i == 7 || i == 8 || i == 15 }

// ANSI16Neutrals reports, per mode, the neutral roles (NeutralRoles) a 16-colour terminal
// draws on a hue, and the pairs of the text ramp (TextRamp) that draw as one grey other
// than by the engine's own assignment (a light page's fgMuted and fgSubtle both on 8),
// after the theme's ansi16 pins. derive puts every neutral on its grey (GreySlot16), so
// only a pin moves one off; palette check fails a neutral on red and warns on the rest,
// each with the pin that puts it back.
func ANSI16Neutrals(f File) []NeutralSlot {
	var out []NeutralSlot
	for _, mode := range modes16(f) {
		dark := mode != "light"
		tc := ResolveProfile(f, dark, false, colorprofile.TrueColor).P
		for _, role := range NeutralRoles {
			if i := ANSI16Index(f, mode, role); i >= 0 && !Grey16(i) {
				out = append(out, NeutralSlot{mode, role, paletteRole(tc, role), i, ""})
			}
		}
		for i, a := range TextRamp {
			for _, b := range TextRamp[i+1:] {
				ia, ib := ANSI16Index(f, mode, a), ANSI16Index(f, mode, b)
				if ia < 0 || ia != ib || (ia == GreySlot16(a, dark) && ib == GreySlot16(b, dark)) {
					continue
				}
				out = append(out, NeutralSlot{mode, b, paletteRole(tc, b), ib, a})
			}
		}
	}
	return out
}

func sameColor(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	r1, g1, b1, _ := a.RGBA()
	r2, g2, b2, _ := b.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2
}

// CheckHex reports whether s is a colour Lip Gloss draws as written: "#RGB" or
// "#RRGGBB", either case. Lip Gloss reads a string without "#" as an ANSI index
// ("12"), a packed integer ("123456") or no colour at all ("6B50FF"), and any
// other length after "#" as no colour, so all of those are errors here.
func CheckHex(s string) error {
	if s == "" {
		return fmt.Errorf("empty colour: write #RGB or #RRGGBB")
	}
	body, hasHash := strings.CutPrefix(s, "#")
	digits := (len(body) == 3 || len(body) == 6) && strings.Trim(body, "0123456789abcdefABCDEF") == ""
	switch {
	case hasHash && digits:
		return nil
	case !strings.Contains(s, "#"):
		fix := "#RGB or #RRGGBB"
		if digits {
			fix = fmt.Sprintf("%q", "#"+s)
		}
		return fmt.Errorf("%q has no #: Lip Gloss reads it as an ANSI index, a packed integer or no colour; write %s", s, fix)
	}
	return fmt.Errorf("%q is not #RGB or #RRGGBB", s)
}

// check validates every non-empty role, naming the role by its JSON key.
func (r Roles) check() error {
	v, t := reflect.ValueOf(r), reflect.TypeOf(r)
	for i := range t.NumField() {
		s := v.Field(i).String()
		if s == "" {
			continue
		}
		if err := CheckHex(s); err != nil {
			key, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	return nil
}

// Palette is a resolved role set as colours.
type Palette struct {
	Primary, Secondary, Tertiary, Accent                  color.Color
	Bg, BgPanel, BgSubtle, BgOverlay                      color.Color
	Fg, FgSoft, FgMuted, FgSubtle, FgSelected             color.Color
	Border, BorderFocus                                   color.Color
	Success, Error, Warning, Info                         color.Color
	OnPrimary, OnSecondary, OnError, OnWarning, OnSuccess color.Color
}

// Theme is a resolved theme: one mode, one glyph set, ready-made styles.
type Theme struct {
	File
	Dark  bool
	ASCII bool
	// Profile is the terminal's colour profile the styles were built for (see
	// ResolveProfile); Unknown means truecolour, the colours as written.
	Profile colorprofile.Profile
	P       Palette
	G       Glyphs
	X       Expressions
	S       Styles
	// Unicode is the glyph set as the theme wrote it (defaults filled), before any ASCII
	// twin: G itself unless ASCII. What a component reads to decide by the theme's own
	// glyphs rather than their twins (ui.BusyAnimates judges the spinner frames' width).
	Unicode Glyphs
	// KeyDisambiguation is what the terminal reported in tea.KeyboardEnhancementsMsg
	// (SupportsKeyDisambiguation): shift+enter arrives as itself, not as enter. ui.Help
	// reads it to draw "shift+enter/ctrl+j" as the one alternative that works here.
	// Resolve leaves it false (ctrl+j, which every terminal sends); set it again after
	// every Resolve once the message has arrived.
	KeyDisambiguation bool

	promptTertiary bool // the focus ink doesn't read on bgOverlay: the prompt keeps tertiary (build)
	plainBrand     bool // primary is pinned to a grey at 16 colours: no hue, so the brand is fg and bold (build)
}

// Resolve picks the dark or light roles, fills gaps, and builds every style, for a
// truecolour terminal. Call it on startup and again when tea.BackgroundColorMsg
// arrives; an app that renders to other terminals calls ResolveProfile.
func Resolve(f File, dark, ascii bool) *Theme {
	return ResolveProfile(f, dark, ascii, colorprofile.TrueColor)
}

// ResolveProfile is Resolve for the colour profile the terminal reports
// (tea.ColorProfileMsg: 16 colours, 256, or none under NO_COLOR). At 16 colours every
// neutral role draws on one of the four greys (GreySlot16: the page 0 or 15, body text
// the opposite, fgMuted and fgSubtle on 7 and 8) and the theme's ansi16 pins replace
// the roles they name; a primary pinned to a grey has no hue, and the kit's focus draws
// as under expressions.focus "fg" (fg and bold), brand text in fg and bold. Where a
// background step the styles separate things with can't be told from the page at
// that profile (Distinct), they add an edge instead: a borderless dialog gets a rule
// above and below. A rule or border colour that lands on bg moves to the nearest one
// that doesn't (ANSI 8 or 7 at 16 colours), so rules stay visible. Keep the profile and pass it again on every re-resolve.
func ResolveProfile(f File, dark, ascii bool, profile colorprofile.Profile) *Theme {
	mode := "dark"
	r, ok := f.Roles["dark"]
	if !dark {
		if l, ok2 := f.Roles["light"]; ok2 {
			r, ok, mode = l, true, "light"
		}
	}
	if !ok {
		r = Default().Roles["dark"]
	}
	t := &Theme{File: f, Dark: dark, ASCII: ascii, Profile: profile}
	var pins map[string]int
	if profile == colorprofile.ANSI {
		pins = pins16(f, mode, dark)
	}
	t.P = derive(r, dark, pins)
	// the prompt takes the focus ink, unless that ink is under 3:1 on bgOverlay, where
	// a palette's filter input sits (Charm's #6B50FF on #4D4C57 is 1.7:1): judged on the
	// colours as written, so one theme and mode draws one prompt colour in every profile
	tc := t.P
	if pins != nil {
		tc = derive(r, dark, nil)
	}
	t.promptTertiary = Contrast(tc.Primary, tc.BgOverlay) < promptFloor
	t.Unicode = glyphDefaults(f.Glyphs)
	t.G = t.Unicode
	if ascii {
		t.G = ASCII(t.G)
	}
	t.X = exprDefaults(f.Expressions)
	// primary pinned to a grey at 16 colours has no hue there (a warm brand on a light
	// page: yellow 3 is unreadable on white, magenta is pink): the kit's emphasis falls
	// back as under focus "fg", and brand text is fg and bold
	if n, ok := pins["primary"]; ok && Grey16(n) {
		t.X.Focus, t.plainBrand = "fg", true
	}
	if t.X.Focus == "fg" && r.BorderFocus == "" {
		t.P.BorderFocus = t.P.FgMuted // the dialog border is chrome too: not the hue
	}
	// rules are "barely lighter than bg": downsampled, they can land on bg itself and
	// every section, table and footer rule vanishes
	t.P.Border, t.P.BorderFocus = t.edge(t.P.Border), t.edge(t.P.BorderFocus)
	// a pin moves a fill to a slot its ink was never chosen for: halyard's dark ink on a
	// primary pinned to fg's black (a warm brand on a light page) is black on black in
	// the header bar, the dialog title and a filled tag. Each ink that lands on its
	// fill's slot takes fg or bg instead (Ink), once, for every component that draws it
	if pins != nil {
		p := &t.P
		p.OnPrimary, p.FgSelected = t.Ink(p.OnPrimary, p.Primary), t.Ink(p.FgSelected, p.Primary)
		p.OnSecondary, p.OnError = t.Ink(p.OnSecondary, p.Secondary), t.Ink(p.OnError, p.Error)
		p.OnWarning, p.OnSuccess = t.Ink(p.OnWarning, p.Warning), t.Ink(p.OnSuccess, p.Success)
	}
	t.S = build(t)
	return t
}

// Resolve16 is ResolveProfile at 16 colours (colorprofile.ANSI), without ASCII: what a
// checker judges a theme's ansi16 pins by.
func Resolve16(f File, dark bool) *Theme {
	return ResolveProfile(f, dark, false, colorprofile.ANSI)
}

// Distinct reports whether a and b draw as different colours at the theme's profile:
// under 16 or 256 colours two hexes can land on one index, and under NO_COLOR (or
// no TTY) nothing has a colour at all.
func (t *Theme) Distinct(a, b color.Color) bool {
	p := t.Profile
	if p == colorprofile.Unknown {
		p = colorprofile.TrueColor
	}
	ca, cb := p.Convert(a), p.Convert(b)
	if ca == nil || cb == nil {
		return ca != cb
	}
	r1, g1, b1, _ := ca.RGBA()
	r2, g2, b2, _ := cb.RGBA()
	return r1 != r2 || g1 != g2 || b1 != b2
}

// edge is c, a rule or border colour, or where c lands on the page's colour at the
// theme's profile, the nearest colour that doesn't: at 16 colours the quiet grey (ANSI 8
// on a dark page, 7 on a light one, whichever is apart from bg), at 256 c stepped toward
// fg until it leaves bg's slot. Like Ink, a no-op wherever the two stay apart, and under
// NO_COLOR, where nothing has a colour and a rule is its glyphs.
func (t *Theme) edge(c color.Color) color.Color {
	if t.Profile != colorprofile.ANSI && t.Profile != colorprofile.ANSI256 || t.Distinct(c, t.P.Bg) {
		return c
	}
	if t.Profile == colorprofile.ANSI {
		greys := []int{8, 7}
		if !t.Dark {
			greys = []int{7, 8}
		}
		for _, n := range greys {
			if g := ansi.BasicColor(n); t.Distinct(g, t.P.Bg) {
				return g
			}
		}
		return c
	}
	for i := 1; i <= 20; i++ {
		if m := Mix(c, t.P.Fg, float64(i)/20); t.Distinct(m, t.P.Bg) {
			return m
		}
	}
	return c
}

// Ink is the colour to draw text on fill with: c, or when c and fill land on one
// colour at the theme's profile, fg or bg, whichever stays apart from the fill. At
// truecolour, and wherever the two stay apart, it is c.
func (t *Theme) Ink(c, fill color.Color) color.Color {
	if t.Distinct(c, fill) {
		return c
	}
	for _, alt := range []color.Color{t.P.Fg, t.P.Bg} {
		if t.Distinct(alt, fill) {
			return alt
		}
	}
	return c
}

// Role is the resolved colour of the role with JSON key key (primary, fgSubtle, …), or
// nil when key names none: what a ladder rung's role draws in.
func (t *Theme) Role(key string) color.Color {
	if !roleKey(key) {
		return nil
	}
	return paletteRole(t.P, key)
}

// LadderGlyph is the glyph rung r draws in this theme: the slot's glyph when r.Glyph
// names a status slot, else the literal; under ASCII, its twin either way.
func (t *Theme) LadderGlyph(r LadderRung) string {
	if g, ok := GlyphSlot(t.G, r.Glyph); ok {
		return g
	}
	if t.ASCII {
		return twin(t.File.Glyphs.ASCII, r.Glyph)
	}
	return r.Glyph
}

// BlurBar is the marker a blurred selection keeps (ui.List, ui.Table): glyphs.blurBar,
// or when the theme leaves it unset ╎ (ASCII :). It is never the focus bar's glyph:
// a blurred and a focused selection differ in shape, so NO_COLOR and ASCII still show
// one focus (CheckGlyphs refuses a blurBar that is the focusBar).
func (t *Theme) BlurBar() string {
	if t.G.BlurBar != "" {
		return t.G.BlurBar
	}
	if t.ASCII {
		return blurFallbackASCII
	}
	return blurFallback
}

// AppName is the brand name the theme declares.
func (t *Theme) AppName() string {
	if t.Brand != "" {
		return t.Brand
	}
	return "app"
}

// Hex parses "#rrggbb" or "#rgb" into a colour (nil for ""). It does not
// validate: see CheckHex.
func Hex(s string) color.Color {
	if s == "" {
		return nil
	}
	return lipgloss.Color(s)
}
