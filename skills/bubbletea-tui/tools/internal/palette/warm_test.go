package palette

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tuikit/theme"
	"tuitools/internal/oklch"
)

// A warm brand at 16 colours (#51), swept over the hue circle: derive's output for a
// brand at OKLCH L 0.62 C 0.15, every 15° and every 2° from 16° to 34° (the reds,
// vermilions and corals around ANSI red), on a dark page and a light one, with derive's
// own pins. For every hue:
//
//   - the warning draws on yellow (3 or 11), never on green, blue or cyan;
//   - a warm brand never draws on magenta, nor its steps (secondary, tertiary, accent)
//     on error's red or magenta;
//   - the brand shares the warning's hue only as a warm brand on a dark page, on 11
//     beside the warning's 3, where the warning's glyph tells them apart;
//   - a pinned primary reads as text (3:1) and under its inks (4.5:1) in cellshot's
//     palette (Inks16), and every other pin reads as text at 3:1, but for the warning's
//     yellow 3 on a light page, which is 1.35:1 there: the warning keeps yellow and its
//     glyph (▲ !), as every terminal palette means it;
//   - the pins leave nothing for check to report.
//
// Before #51 the oranges and coppers (30–45°) drew their brand on magenta 13/5, and the
// ambers (60–75°) put a light warning on cyan 6, info's.
func TestWarmHueSweep(t *testing.T) {
	var hues []float64
	for h := 0.0; h < 360; h += 15 {
		hues = append(hues, h)
	}
	for h := 16.0; h <= 34; h += 2 {
		hues = append(hues, h)
	}
	text := func(n, bg int) float64 { return oklch.Contrast(basic16Hex(n), basic16Hex(bg)) }
	var table []string
	for _, h := range hues {
		hex := oklch.FromLCH(0.62, 0.15, h)
		f := themeOf(Derive(hex, "", "", "#1C1917", "#FAF7F2", 0.012))
		f.ANSI16 = Pins16(f)
		row := fmt.Sprintf("%3.0f° %s", h, hex)
		for _, mode := range []string{"dark", "light"} {
			_, _, hp := oklch.LCH(f.Roles[mode].Primary)
			p, w := theme.ANSI16Index(f, mode, "primary"), theme.ANSI16Index(f, mode, "warning")
			s, i := theme.ANSI16Index(f, mode, "success"), theme.ANSI16Index(f, mode, "info")
			where := fmt.Sprintf("hue %.0f° (%s, %s primary %s at %.0f°)", h, hex, mode, f.Roles[mode].Primary, hp)
			if w != 3 && w != 11 {
				t.Errorf("%s: the warning draws as ANSI %d, want yellow 3 or 11", where, w)
			}
			switch w % 8 {
			case 2, 4, 6:
				t.Errorf("%s: the warning draws as ANSI %d, a status hue (green, blue or cyan)", where, w)
			}
			if warm(hp) && (p == 5 || p == 13) {
				t.Errorf("%s: the warm brand draws as ANSI %d, magenta", where, p)
			}
			for _, role := range []string{"secondary", "tertiary", "accent"} {
				if n := theme.ANSI16Index(f, mode, role); warm(hp) && (n == 1 || n == 9 || n == 5 || n == 13) {
					t.Errorf("%s: the warm brand's %s draws as ANSI %d, red or magenta", where, role, n)
				}
			}
			shared := ""
			if p%8 == w%8 {
				shared = " shares the warning's hue"
				if !(warm(hp) && mode == "dark" && p == 11 && w == 3) {
					t.Errorf("%s: the brand (ANSI %d) shares the warning's hue (ANSI %d) outside a warm brand on a dark page", where, p, w)
				}
			}
			bg := theme.ANSI16Index(f, mode, "bg")
			for role, n := range f.ANSI16[mode] {
				if role == "primary" || (role == "warning" && mode == "light" && n == 3) {
					continue // primary is Inks16's below; the light warning's yellow is the rule's
				}
				if r := text(n, bg); r < 3 {
					t.Errorf("%s: %s pinned to ANSI %d reads %.2f:1 on the page's ANSI %d, under 3", where, role, n, r, bg)
				}
			}
			row += fmt.Sprintf(" | %s P%-2d W%-2d S%-2d I%-2d%s", mode[:1], p, w, s, i, shared)
		}
		for _, x := range Inks16(f) {
			t.Errorf("hue %.0f° (%s): %s", h, hex, x)
		}
		for _, x := range Findings16(f) {
			t.Errorf("hue %.0f° (%s): derive's pins leave %s", h, hex, x.What)
		}
		table = append(table, row)
	}
	t.Logf("brand hue | dark P/W/S/I | light P/W/S/I\n%s", strings.Join(table, "\n"))
}

// Coral #D15D51 is OKLCH 28°, 1° under ANSI red's 29°, where the first attempt's warm arc
// began: it still drew on magenta. It is warm: yellow 11 on a dark page, no hue (fg's
// grey) on a light one; a crimson at 20° is a red, and keeps the nearest free slot.
func TestCoralIsWarm(t *testing.T) {
	_, _, h := oklch.LCH("#D15D51")
	if h < 27 || h > 29 {
		t.Fatalf("coral is %.1f°, want 28°: the probe no longer sits under ANSI red", h)
	}
	f := themeOf(Derive("#D15D51", "", "", "#1C1917", "#FAF7F2", 0.012))
	pins := Pins16(f)
	if pins["dark"]["primary"] != 11 || pins["light"]["primary"] != 0 {
		t.Errorf("coral pins %v, want dark primary 11 and light primary 0", pins)
	}
	for mode, want := range map[string]bool{"dark": true, "light": true} {
		if _, ok := WarmPin16(f, mode); ok != want {
			t.Errorf("coral %s: warm %v", mode, ok)
		}
	}
	crimson := theme.File{Roles: map[string]theme.Roles{"dark": {Bg: "#1C1917", Fg: "#E8E2D6", Primary: "#D94060"}}}
	if _, ok := WarmPin16(crimson, "dark"); ok {
		t.Error("crimson #D94060 (13°) is warm: a red's nearest free slot is magenta's pink, which suits it")
	}
}

// palette check judges a primary pin at the 16-colour slot it draws on, in the palette
// the sheets show (cellshot's), not only at truecolour, and fails each case #51's table
// found with every check passing: white on bright yellow (ferrous's selection), a warning
// on green, success's slot (ferrous's light warning), and yellow 3 brand text on white.
func TestCheckFailsIssue51Table(t *testing.T) {
	dark := `"dark": {"bg": "#1A1B1D", "fg": "#D7D9DC", "primary": "#E25317", "fgSelected": "#FFFFFF", "success": "#6E9E78", "error": "#E0453A", "warning": "#CFA24E", "info": "#8C98A6"}`
	light := `"light": {"bg": "#FAF9F7", "fg": "#141516", "primary": "#B33A08", "fgSelected": "#FFFFFF", "success": "#2A6E36", "error": "#B71C1C", "warning": "#7A5200", "info": "#3C4A58"}`
	for _, c := range []struct {
		name, roles, pins, want string
	}{
		{"white on bright yellow", dark, `{"dark": {"primary": 11, "warning": 3}}`,
			`dark: fgSelected #FFFFFF draws as ANSI 15 on primary's ANSI 11 at 16 colours (-profile ansi), 1.17:1 in cellshot's palette, under 4.5`},
		{"warning on green", light, `{"light": {"primary": 0, "warning": 2}}`,
			`light: warning #7A5200 draws as ANSI 2 at 16 colours (-profile ansi), success's green, so a warning reads as success there; the warning keeps yellow: pin it with "ansi16": {"light": {"warning": 3}}`},
		{"yellow 3 brand text on white", light, `{"light": {"primary": 3, "warning": 6}}`,
			`light: primary #B33A08 draws as ANSI 3 on bg's ANSI 15 at 16 colours (-profile ansi), 1.35:1 in cellshot's palette, under 3.0`},
	} {
		path := filepath.Join(t.TempDir(), "theme.json")
		if err := os.WriteFile(path, []byte(`{"roles": {`+c.roles+`}, "ansi16": `+c.pins+`}`), 0o644); err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		if code := Main([]string{"check", path}, "", &out, &errb); code != 1 {
			t.Errorf("%s: exit %d, want 1\n%s", c.name, code, errb.String())
		}
		if !strings.Contains(errb.String(), c.want) {
			t.Errorf("%s: stderr %q, want %q", c.name, errb.String(), c.want)
		}
	}
}

// The kit's themes read at 16 colours with nothing for check to say: ferrous's selection
// ink is dark on its bright yellow, and neither orange brand (ferrous, halyard) draws on
// magenta, nor its warning off yellow; quire's amber, a state under focus "fg", has no
// hue on a light page either.
func TestKitThemesReadAt16Colours(t *testing.T) {
	kit, _ := filepath.Glob(kitThemes + "/*.json")
	if len(kit) == 0 {
		t.Fatal("no kit themes")
	}
	for _, path := range append(kit, kitThemes+"/../theme/testdata/quire.json") {
		f, err := theme.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range Inks16(f) {
			t.Errorf("%s: %s", filepath.Base(path), x)
		}
		for _, x := range Findings16(f) {
			t.Errorf("%s: %s; %s", filepath.Base(path), x.What, x.PinJSON())
		}
	}
}
