package theme

import (
	"path/filepath"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// A primary pinned to a grey at 16 colours has no hue there (a warm brand on a light
// page, #51: yellow 3 is 1.35:1 on white, magenta is the pink an orange is not). The
// kit's emphasis then falls back as under focus "fg": the focus, prompt and focus bar
// are fg and bold, a fill selection is fg reversed, and brand text is fg and bold, so
// it still stands apart from body text by weight. Truecolour and 256 colours keep the
// hue and the theme's own focus.
func TestGreyPrimaryPinDropsTheHue(t *testing.T) {
	f := File{
		Roles:       map[string]Roles{"light": {Bg: "#FAF9F7", Fg: "#141516", Primary: "#B33A08", FgSelected: "#FFFFFF"}},
		Expressions: Expressions{Selection: "fill"},
		ANSI16:      map[string]map[string]int{"light": {"primary": 0}},
	}
	th := ResolveProfile(f, false, false, colorprofile.ANSI)
	if th.X.Focus != "fg" {
		t.Errorf("light at 16 colours: focus %q, want fg (primary has no hue there)", th.X.Focus)
	}
	for name, st := range map[string]lipgloss.Style{"Focus": th.S.Focus, "Prompt": th.S.Prompt, "FocusBar": th.S.FocusBar, "Primary": th.S.Primary} {
		if !st.GetBold() {
			t.Errorf("S.%s is not bold: with no hue, weight is the brand's cue", name)
		}
		if n := Slot16(st.GetForeground()); n != 0 {
			t.Errorf("S.%s draws as ANSI %d, want fg's 0", name, n)
		}
	}
	if fg := Slot16(th.S.SelRow.GetForeground()); fg != 0 || !th.S.SelRow.GetReverse() {
		t.Errorf("fill selection draws ANSI %d (reverse %v), want fg reversed", fg, th.S.SelRow.GetReverse())
	}
	for _, p := range []colorprofile.Profile{colorprofile.TrueColor, colorprofile.ANSI256} {
		if th := ResolveProfile(f, false, false, p); th.X.Focus != "primary" || th.S.Primary.GetBold() {
			t.Errorf("%v: focus %q, bold brand %v: the hue returns past 16 colours", p, th.X.Focus, th.S.Primary.GetBold())
		}
	}
}

// The kit's two orange brands at 16 colours (#51). Before, both drew their brand in
// magenta 13/5, the pink an orange's read forbids, and ferrous's white selection ink sat
// on it at 2.94:1. A warm brand takes bright yellow 11 on a dark page, with the warning
// on the other yellow, 3, and a dark ink on the selection bar; on a light page it has no
// hue, never yellow 3 on white and never magenta, and the warning keeps yellow 3.
func TestWarmKitThemesAt16Colours(t *testing.T) {
	for _, name := range []string{"ferrous", "halyard"} {
		f, err := Load(filepath.Join("..", "themes", name+".json"))
		if err != nil {
			t.Skipf("%s: %v (a copied kit keeps only its own theme)", name, err)
		}
		for _, c := range []struct {
			dark             bool
			primary, warning int
			focus            string
		}{{true, 11, 3, "primary"}, {false, 0, 3, "fg"}} {
			th := ResolveProfile(f, c.dark, false, colorprofile.ANSI)
			if p := Slot16(th.P.Primary); p != c.primary {
				t.Errorf("%s dark=%v: primary draws as ANSI %d, want %d", name, c.dark, p, c.primary)
			}
			if w := Slot16(th.P.Warning); w != c.warning {
				t.Errorf("%s dark=%v: warning draws as ANSI %d, want yellow %d", name, c.dark, w, c.warning)
			}
			if th.X.Focus != c.focus {
				t.Errorf("%s dark=%v: focus %q, want %q", name, c.dark, th.X.Focus, c.focus)
			}
			// nor its steps: ferrous's secondary and halyard's accent sat on error's red
			for role, st := range map[string]lipgloss.Style{"Focus": th.S.Focus, "Primary": th.S.Primary, "FocusBar": th.S.FocusBar, "Title": th.S.Title,
				"Secondary": th.S.Secondary, "Tertiary": th.S.Tertiary, "Accent": th.S.Accent} {
				if n := Slot16(st.GetForeground()); n == 5 || n == 13 || n == 1 || n == 9 || (!c.dark && n == 3) {
					t.Errorf("%s dark=%v: S.%s draws as ANSI %d", name, c.dark, role, n)
				}
			}
			if c.dark { // the selection bar: reverse of (primary, ink), so the ink is the background
				if ink := Slot16(th.S.SelRow.GetBackground()); ink != 0 {
					t.Errorf("%s dark: the selection ink draws as ANSI %d on bright yellow, want black 0", name, ink)
				}
			}
		}
	}
}
