package palette

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"tuikit/theme"
	"tuitools/internal/oklch"
)

// The goldens in testdata are scripts/palette.py's output on the same inputs:
//
//	check/<name>.txt     stdout of `palette.py check` on kit/themes/<name>.json or check/<name>.json
//	derive/<case>.*      `palette.py derive <args> --swatch swatch.ansi` run in an empty
//	                     directory: .out stdout, .err stderr, .ansi the swatch, .json the --out file
//	derive/corpus.txt    roles and xterm indices for 250 random inputs (of 8,000 matched)
//
// The intended differences: the swatch hint names the Go cellshot, not cellshot.py;
// derive writes the "ansi16" pins its own check would ask for (and says so on stderr),
// and from one brand hue makes secondary and tertiary lightness steps of it instead of
// a hue shift and a complement, so the derive/* goldens were regenerated from the Go
// tool when that changed (the one-hex cases one-hue, tint, near-grey and very-light,
// and every case's ansi16 block; corpus.txt keeps palette.py's rows, with the one-hex
// rows' secondary, tertiary and onSecondary compared against the new rule instead);
// check fails role colours Lip Gloss would misread, which palette.py let through (see
// TestCheckRejectsWhatTheEngineRejects); and where neither ink cleared an on-colour's
// floor (palette.py shipped the failure), the ink is now theme.OnInk's deepened one.
// That changed derive/tint and derive/tint-zero (exit 1 → 0), 14 corpus rows'
// onPrimary/fgSelected, and check/failing's light onPrimary (the engine now draws
// #444444, not #555555, on #7FB3FF). The border floor moved from 1.14 to 1.3 with a
// warning under 1.5, and derive's border from a 0.07 L step (1.2:1) to 1.5:1
// (TestBorderFloor): every check/*.txt border line, every derive/* border (its .out,
// .err and the swatch's rule row) and the kit themes' own borders were regenerated for
// that alone; corpus.txt keeps palette.py's rows, with border compared against the new
// rule instead.

const kitThemes = "../../../kit/themes"

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func golden(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCheckMatchesPython(t *testing.T) {
	var exits map[string]int
	readJSON(t, "testdata/check/exits.json", &exits)
	kit, _ := filepath.Glob(kitThemes + "/*.json")
	if len(kit) == 0 {
		t.Fatal("no kit themes")
	}
	for _, f := range kit { // every kit theme has a golden
		if _, ok := exits[strings.TrimSuffix(filepath.Base(f), ".json")]; !ok {
			t.Errorf("no golden for %s", f)
		}
	}
	for name, exit := range exits {
		path := filepath.Join(kitThemes, name+".json")
		if _, err := os.Stat(path); err != nil {
			path = filepath.Join("testdata/check", name+".json")
		}
		var out, errb bytes.Buffer
		code := Main([]string{"check", path}, "", &out, &errb)
		if code != exit {
			t.Errorf("%s: exit %d, palette.py exits %d (stderr %q)", name, code, exit, errb.String())
		}
		if want := golden(t, "testdata/check/"+name+".txt"); out.String() != want {
			t.Errorf("%s: output differs from palette.py\n--- go\n%s\n--- python\n%s", name, out.String(), want)
		}
	}
}

func TestDeriveMatchesPython(t *testing.T) {
	var cases []struct {
		Name string
		Args []string
	}
	readJSON(t, "testdata/derive/cases.json", &cases)
	var exits map[string]int
	readJSON(t, "testdata/derive/exits.json", &exits)
	td, err := filepath.Abs("testdata/derive")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	for _, c := range cases {
		base := filepath.Join(td, c.Name)
		var out, errb bytes.Buffer
		code := Main(append([]string{"derive"}, append(c.Args, "--swatch", "swatch.ansi")...), "", &out, &errb)
		if code != exits[c.Name] {
			t.Errorf("%s: exit %d, palette.py exits %d", c.Name, code, exits[c.Name])
		}
		if want := golden(t, base+".out"); out.String() != want {
			t.Errorf("%s: stdout differs from palette.py\n--- go\n%s\n--- python\n%s", c.Name, out.String(), want)
		}
		want := strings.Replace(golden(t, base+".err"), "(render: cellshot.py ", "(render: cellshot ", 1)
		if errb.String() != want {
			t.Errorf("%s: stderr differs from palette.py\n--- go\n%s\n--- python\n%s", c.Name, errb.String(), want)
		}
		if got := golden(t, "swatch.ansi"); got != golden(t, base+".ansi") {
			t.Errorf("%s: swatch differs from palette.py", c.Name)
		}
		if _, err := os.Stat(base + ".json"); err == nil {
			if got := golden(t, "roles.json"); got != golden(t, base+".json") {
				t.Errorf("%s: --out file differs from palette.py", c.Name)
			}
		}
	}
}

func TestDeriveCorpusMatchesPython(t *testing.T) {
	f, err := os.Open("testdata/derive/corpus.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	n := 0
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "# ") {
			continue
		}
		parts := strings.Split(sc.Text(), " | ")
		in := strings.Fields(parts[0])
		for i := range 5 {
			if in[i] == "-" {
				in[i] = ""
			}
		}
		tint, err := strconv.ParseFloat(in[5], 64)
		if err != nil {
			t.Fatal(err)
		}
		modes := Derive(in[0], in[1], in[2], in[3], in[4], tint)
		for i, m := range modes {
			keys := make([]string, 0, len(m.Roles))
			for k := range m.Roles {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			hexes, idx := make([]string, len(keys)), make([]string, len(keys))
			pyHex, pyIdx := strings.Fields(parts[1+2*i]), strings.Fields(parts[2+2*i])
			for j, k := range keys {
				hexes[j], idx[j] = m.Roles[k], strconv.Itoa(oklch.Xterm256(m.Roles[k]))
				// one brand hue: secondary and tertiary are now lightness steps of it, not
				// palette.py's hue shift and complement (TestOneHueStepsLightness); the
				// rest of the set still matches
				if in[1] == "" && (k == "secondary" || k == "tertiary" || k == "onSecondary") {
					pyHex[j], pyIdx[j] = hexes[j], idx[j]
				}
				// the border now lands at 1.5:1 (TestBorderFloor), not palette.py's step of
				// 0.07 L that read at 1.2, so it is held to the band instead of the row, on
				// every page that lies on its mode's side (a dark --light-bg, where every
				// role tunes toward the wrong end and clamps, is the brand's call, as
				// TestDeriveAlwaysPassesCheck says of a background of the brand's own)
				if k == "border" {
					pyHex[j], pyIdx[j] = hexes[j], idx[j]
					lb, _, _ := oklch.LCH(m.Roles["bg"])
					if cr := oklch.Contrast(m.Roles[k], m.Roles["bg"]); (lb < 0.5) == (m.Name == "dark") && (cr < 1.5 || cr > 1.9) {
						t.Errorf("derive %v %s: border %s on bg %s is %.2f:1, want 1.5–1.9", in, m.Name, m.Roles[k], m.Roles["bg"], cr)
					}
				}
			}
			if got := strings.Join(hexes, " "); got != strings.Join(pyHex, " ") {
				t.Errorf("derive %v %s roles:\n go %s\n py %s", in, m.Name, got, parts[1+2*i])
			}
			if got := strings.Join(idx, " "); got != strings.Join(pyIdx, " ") {
				t.Errorf("derive %v %s ansi256:\n go %s\n py %s", in, m.Name, got, parts[2+2*i])
			}
		}
		n++
	}
	if n < 250 {
		t.Fatalf("only %d corpus rows", n)
	}
}

// The checker's on-colours are the engine's: for every kit theme and mode, a filled
// on-colour is exactly what theme.Resolve draws.
func TestFillOnColoursIsTheEngine(t *testing.T) {
	kit, _ := filepath.Glob(kitThemes + "/*.json")
	for _, path := range kit {
		f, err := theme.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		for mode, roles := range f.Roles {
			dark := mode != "light"
			b, _ := json.Marshal(roles)
			r := map[string]string{}
			_ = json.Unmarshal(b, &r)
			for k, v := range r {
				if v == "" {
					delete(r, k)
				}
			}
			if err := FillOnColours(r, dark); err != nil {
				t.Fatal(err)
			}
			p := theme.Resolve(f, dark, false).P
			for on, c := range map[string]color.Color{"onPrimary": p.OnPrimary, "onSecondary": p.OnSecondary,
				"onWarning": p.OnWarning, "onError": p.OnError, "onSuccess": p.OnSuccess, "fgSelected": p.FgSelected} {
				if !same(oklch.Color(r[on]), c) {
					t.Errorf("%s %s: checker fills %s=%s, engine draws %s", filepath.Base(path), mode, on, r[on], hexOf(c))
				}
			}
		}
	}
}

// A theme without bg gets the engine's default background as a candidate ink (palette.py
// used #000000, which the app never draws).
func TestFillOnColoursUsesEngineDefaults(t *testing.T) {
	r := map[string]string{"primary": "#8FB8DE", "fg": "#D8DEE9"}
	if err := FillOnColours(r, true); err != nil {
		t.Fatal(err)
	}
	if r["onPrimary"] != "#15171C" || r["fgSelected"] != "#15171C" {
		t.Errorf("onPrimary %s, fgSelected %s; want the engine's dark bg #15171C", r["onPrimary"], r["fgSelected"])
	}
	if _, ok := r["onSecondary"]; ok {
		t.Errorf("onSecondary filled without a secondary")
	}
}

func TestUsageAndErrors(t *testing.T) {
	for _, c := range []struct {
		args []string
		exit int
		out  string
	}{
		{nil, 0, "USAGE"}, {[]string{"-h"}, 0, "USAGE"}, {[]string{"--help"}, 0, "USAGE"},
		{[]string{"paint"}, 2, "USAGE"}, {[]string{"check"}, 2, ""}, {[]string{"check", "nope.json"}, 2, ""},
		{[]string{"derive"}, 2, ""}, {[]string{"derive", "red"}, 2, ""}, {[]string{"derive", "#123456", "--bg"}, 2, ""},
		{[]string{"derive", "#123456", "--tint", "lots"}, 2, ""}, {[]string{"derive", "#123456", "#xyz"}, 2, ""},
	} {
		var out, errb bytes.Buffer
		if code := Main(c.args, "USAGE", &out, &errb); code != c.exit || strings.TrimSpace(out.String()) != c.out {
			t.Errorf("palette %v: exit %d stdout %q; want %d %q", c.args, code, out.String(), c.exit, c.out)
		}
	}
}

func TestOptionWithEquals(t *testing.T) {
	var a, b, e bytes.Buffer
	Main([]string{"derive", "#6B50FF", "--bg=#201F26", "--name=x"}, "", &a, &e)
	Main([]string{"derive", "#6B50FF", "--bg", "#201F26", "--name", "x"}, "", &b, &e)
	if a.String() != b.String() || !strings.Contains(a.String(), `"bg": "#201F26"`) {
		t.Errorf("--opt=value and --opt value differ")
	}
}

// A deliberate break from palette.py, which read colours as loosely as its hex2rgb
// does: "6B50FF", "#6B50", "#6B50F" and "#6B50FFAA" all passed. Lip Gloss reads a
// string without "#" as an ANSI index or no colour, and any other length after "#" as
// no colour, so the app drew something else while the check said OK. check now uses
// the engine's rule (theme.CheckHex): it fails exactly the themes theme.Load fails,
// with one line on stderr and exit 2.
func TestCheckRejectsWhatTheEngineRejects(t *testing.T) {
	for _, c := range []struct {
		roles string
		err   string // the stderr line after "palette: <path>: "; "" means accepted
	}{
		{`"primary": "6B50FF"`, `dark.primary: "6B50FF" has no #: Lip Gloss reads it as an ANSI index, a packed integer or no colour; write "#6B50FF"`},
		{`"primary": "#6B50FFAA"`, `dark.primary: "#6B50FFAA" is not #RGB or #RRGGBB`},
		{`"primary": "#6B50"`, `dark.primary: "#6B50" is not #RGB or #RRGGBB`},
		{`"primary": "#6B50F"`, `dark.primary: "#6B50F" is not #RGB or #RRGGBB`},
		{`"primary": "##6B50FF"`, `dark.primary: "##6B50FF" is not #RGB or #RRGGBB`},
		{`"primary": " #6B50FF"`, `dark.primary: " #6B50FF" is not #RGB or #RRGGBB`},
		{`"warning": "12"`, `dark.warning: "12" has no #: Lip Gloss reads it as an ANSI index, a packed integer or no colour; write #RGB or #RRGGBB`},
		{`"fgSoft": "6B50FF"`, `dark.fgSoft: "6B50FF" has no #: Lip Gloss reads it as an ANSI index, a packed integer or no colour; write "#6B50FF"`},
		{`"primary": "#6B50FF"`, ""},
		{`"primary": "#6b50ff"`, ""},
		{`"primary": "#abc"`, ""},
		{`"primary": ""`, ""},
		{`"primary": "#6B50FF", "bgOverlay": ""`, ""},
	} {
		path := filepath.Join(t.TempDir(), "theme.json")
		js := `{"roles": {"dark": {"bg": "#201F26", "fg": "#DFDBDD", ` + c.roles + `}}}`
		if err := os.WriteFile(path, []byte(js), 0o644); err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		code := Main([]string{"check", path}, "", &out, &errb)
		_, loadErr := theme.Load(path)
		if c.err == "" {
			if code == 2 || loadErr != nil {
				t.Errorf("%s: check exit %d (%q), theme.Load %v; want both to accept", c.roles, code, errb.String(), loadErr)
			}
			continue
		}
		if want := "palette: " + path + ": " + c.err + "\n"; code != 2 || errb.String() != want || out.Len() != 0 {
			t.Errorf("%s: exit %d stderr %q stdout %q; want 2 and %q", c.roles, code, errb.String(), out.String(), want)
		}
		if loadErr == nil {
			t.Errorf("%s: palette check fails it but theme.Load accepts it", c.roles)
		}
	}
}

// check fails a glyph -ascii can't draw, and two states that share a twin (CheckGlyphs).
func TestCheckRejectsGlyphsWithNoASCIITwin(t *testing.T) {
	for _, c := range []struct{ glyphs, err string }{
		{`{"pending": "◔"}`, `glyphs.pending "◔" has no ASCII twin`},
		{`{"pending": "◔", "ascii": {"◔": "◑"}}`, `glyphs.ascii: "◔" → "◑" is not ASCII`},
		{`{"pending": "◔", "ascii": {"◔": "*"}}`, `glyphs.current "●" and glyphs.pending "◔" both draw as "*"`},
		{`{"pending": "◔", "ascii": {"◔": "~"}}`, ""},
		{`{"todo": "◦"}`, ""}, // the kit's map has it
		// under selection "caret" the prompt may not be the cursor (CheckSelectionGlyphs)
		{`{"cursor": "›", "prompt": "›"}, "expressions": {"selection": "caret"}`, `glyphs.prompt "›" is glyphs.cursor under selection "caret"`},
		{`{"cursor": "›", "prompt": "»"}, "expressions": {"selection": "caret"}`, ""},
		{`{"cursor": "›", "prompt": "›"}, "expressions": {"selection": "bar"}`, ""},
	} {
		path := filepath.Join(t.TempDir(), "theme.json")
		js := `{"roles": {"dark": {"bg": "#201F26", "fg": "#DFDBDD"}}, "glyphs": ` + c.glyphs + `}`
		if err := os.WriteFile(path, []byte(js), 0o644); err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		code := Main([]string{"check", path}, "", &out, &errb)
		// theme.Load never fails on a glyph: an app must start; palette check is the gate
		if _, err := theme.Load(path); err != nil {
			t.Errorf("%s: theme.Load %v, want it to load", c.glyphs, err)
		}
		if c.err == "" {
			if code == 2 {
				t.Errorf("%s: check exit 2 (%q), want it to accept", c.glyphs, errb.String())
			}
			continue
		}
		if code != 2 || !strings.Contains(errb.String(), c.err) {
			t.Errorf("%s: check exit %d (%q), want exit 2 with %q", c.glyphs, code, errb.String(), c.err)
		}
	}
}

// Under "focus": "fg" the selection is never a primary fill, so check doesn't ask
// fgSelected to read on primary; primary on bg and onPrimary on primary still count.
func TestFocusFgDropsTheSelectedFillCheck(t *testing.T) {
	roles := `{"dark": {"bg": "#17181B", "fg": "#CFCDCA", "primary": "#E8A33D", "fgSelected": "#E0B060"}}`
	for _, c := range []struct {
		focus      string
		code       int
		selChecked bool
	}{{"primary", 1, true}, {"fg", 0, false}} {
		path := filepath.Join(t.TempDir(), "theme.json")
		// the pin keeps this test on its subject: under focus "fg" the amber would
		// otherwise share ANSI 9 with the default error and fail as a 16-colour clash
		js := `{"roles": ` + roles + `, "expressions": {"focus": "` + c.focus + `"}, "ansi16": {"dark": {"primary": 11}}}`
		if err := os.WriteFile(path, []byte(js), 0o644); err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		code := Main([]string{"check", path}, "", &out, &errb)
		if code != c.code || strings.Contains(out.String(), "fgSelected   on primary") != c.selChecked ||
			!strings.Contains(out.String(), "primary      on bg") || !strings.Contains(out.String(), "onPrimary    on primary") {
			t.Errorf("focus %s: exit %d, want %d\n%s%s", c.focus, code, c.code, out.String(), errb.String())
		}
	}
}

// At 16 colours colorprofile maps a one-hue brand's amber and its error to one ANSI colour (issue
// #12: #D99A3B and #EF8482 both 9 on dark, #AF7400 and #A13E40 both 1 on light). Under
// focus "fg" the amber is a state, so check fails the clash and names the fix, an
// ansi16 pin; with the pin it passes. A clash between the status roles is a warning on
// stderr: the glyph and the word still tell the states apart, and which of the 16
// colours reads is the terminal's palette, not the theme's.
func TestCheckFailsStateClashesAt16Colours(t *testing.T) {
	roles := `{"dark": {"bg": "#17181B", "fg": "#CFCDCA", "primary": "#D99A3B", "error": "#EF8482", "warning": "#F28C6B", "success": "#6ACB82", "info": "#56BDFF"},
	 "light": {"bg": "#FAF9F6", "fg": "#333230", "primary": "#AF7400", "error": "#A13E40", "warning": "#A8481F", "success": "#007835", "info": "#006A9D"}}`
	for _, c := range []struct {
		name, extra string
		code, fails int
		stderr      []string
	}{
		// the amber is warm (WarmPin16): off red it takes yellow 11 on a dark page, and
		// on a light one no hue (fg's grey 0), whatever the focus
		{"focus fg, no pin", `, "expressions": {"focus": "fg"}`, 1, 2, []string{
			`dark: primary #D99A3B draws as ANSI 9 at 16 colours (-profile ansi), error's red, so it reads as an error there: pin it with "ansi16": {"dark": {"primary": 11}}`,
			`light: primary #AF7400 draws as ANSI 1 at 16 colours (-profile ansi), error's red, so it reads as an error there: pin it with "ansi16": {"light": {"primary": 0}}`,
			`warning: `, `dark: warning #F28C6B draws as ANSI 9 at 16 colours (-profile ansi), error's red, so it reads as an error there; "ansi16": {"dark": {"warning": 3}} moves it`}},
		{"focus fg, pinned", `, "expressions": {"focus": "fg"}, "ansi16": {"dark": {"primary": 11, "warning": 3}, "light": {"primary": 0, "warning": 3}}`, 0, 0, nil},
		// under focus "primary" the hue is the kit's emphasis: on error's red it is a
		// warning (focus reads as an error there), not a failure; the amber, also on red
		// with the warning, takes the yellow nearest its hue first (PinOrder) and the
		// warning its own slot's other brightness
		{"focus primary", ``, 0, 0, []string{`warning: `, `dark: primary #D99A3B draws as ANSI 9 at 16 colours (-profile ansi), error's red, so focus and selection read as errors there; "ansi16": {"dark": {"primary": 11}} moves it`,
			`dark: warning #F28C6B draws as ANSI 9 at 16 colours (-profile ansi), error's red, so it reads as an error there; "ansi16": {"dark": {"warning": 3}} moves it`}},
		// a clash between two status roles off red: a warning
		{"status clash", `, "ansi16": {"dark": {"primary": 11, "warning": 3, "info": 10}, "light": {"primary": 0, "warning": 3}}`, 0, 0, []string{`warning: `, `dark: info #56BDFF draws as ANSI 10 at 16 colours (-profile ansi), as success #6ACB82 does, so those states differ there only by glyph and word`}},
		// but a warning on green reads as OK: it fails, and its pin is yellow (#51)
		{"warning on green", `, "ansi16": {"dark": {"primary": 11, "warning": 10}, "light": {"primary": 0, "warning": 2}}`, 1, 2, []string{
			`dark: warning #F28C6B draws as ANSI 10 at 16 colours (-profile ansi), success's green, so a warning reads as success there; the warning keeps yellow: pin it with "ansi16": {"dark": {"warning": 3}}`,
			`light: warning #A8481F draws as ANSI 2 at 16 colours (-profile ansi), success's green, so a warning reads as success there; the warning keeps yellow: pin it with "ansi16": {"light": {"warning": 3}}`}},
		// on info's blue or cyan it warns, and moves back to yellow
		{"warning on cyan", `, "ansi16": {"dark": {"primary": 11, "warning": 14}, "light": {"primary": 0, "warning": 6}}`, 0, 0, []string{
			`warning: `, `light: warning #A8481F draws as ANSI 6 at 16 colours (-profile ansi), info's cyan, so a warning reads as info there; the warning keeps yellow; "ansi16": {"light": {"warning": 3}} moves it`}},
	} {
		path := filepath.Join(t.TempDir(), "theme.json")
		if err := os.WriteFile(path, []byte(`{"roles": `+roles+c.extra+`}`), 0o644); err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		code := Main([]string{"check", path}, "", &out, &errb)
		if code != c.code {
			t.Errorf("%s: exit %d, want %d\n%s%s", c.name, code, c.code, out.String(), errb.String())
		}
		for _, want := range c.stderr {
			if !strings.Contains(errb.String(), want) {
				t.Errorf("%s: stderr %q, want %q", c.name, errb.String(), want)
			}
		}
		if c.stderr == nil && errb.String() != "" {
			t.Errorf("%s: stderr %q, want no clash with primary", c.name, errb.String())
		}
		if c.code == 1 && !strings.Contains(out.String(), fmt.Sprintf("FAIL (0 contrast problems, %d problems at 16 colours)", c.fails)) {
			t.Errorf("%s: verdict %q", c.name, out.String())
		}
	}
	// error off red: a grey error beside a red warning reverses the two, so it fails
	// under any focus (quire's light error #B0263F downsamples to ANSI 8)
	path0 := filepath.Join(t.TempDir(), "theme.json")
	if err := os.WriteFile(path0, []byte(`{"roles": {"light": {"bg": "#FAF9F6", "fg": "#333230", "primary": "#006A9D", "error": "#B0263F", "warning": "#A8481F", "success": "#007835", "info": "#006A9D"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out0, err0 bytes.Buffer
	if code := Main([]string{"check", path0}, "", &out0, &err0); code != 1 ||
		!strings.Contains(err0.String(), `light: error #B0263F draws as ANSI 8 at 16 colours (-profile ansi), not red (1 or 9), so a red state beside it reads as the error: pin it with "ansi16": {"light": {"error": 1}}`) {
		t.Errorf("error off red: exit %d %q", code, err0.String())
	}
	// an ansi16 pin that names no role or no colour is an input error, as in theme.Load
	path := filepath.Join(t.TempDir(), "theme.json")
	if err := os.WriteFile(path, []byte(`{"roles": `+roles+`, "ansi16": {"dark": {"primary": 16}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := Main([]string{"check", path}, "", &out, &errb); code != 2 || !strings.Contains(errb.String(), "ansi16.dark.primary: 16 is not one of the 16 colours") {
		t.Errorf("bad pin: exit %d %q", code, errb.String())
	}
}

// An expression value the kit doesn't know (a typo, or a theme written for a newer kit)
// renders the default silently, so check fails it and lists the values the kit draws;
// an expression key it doesn't read fails the same way. theme.Load loads both.
func TestCheckRejectsUnknownExpressions(t *testing.T) {
	for _, c := range []struct{ expressions, err string }{
		{`{"dialog": "rule"}`, `expressions.dialog "rule" is not a value the kit knows: rounded | square | double | heavy | none | rules`},
		{`{"wordmark": "plane", "focus": "fg"}`, `expressions.wordmark "plane" is not a value the kit knows: gradient | solid | inverse | tracked | bracket | upper | plain`},
		{`{"dailog": "rules"}`, `expressions.dailog is not a key the kit reads`},
		{`{"dialog": "rules", "wordmark": "plain", "focus": "fg", "gutter": 1, "paintBg": true}`, ""},
	} {
		path := filepath.Join(t.TempDir(), "theme.json")
		js := `{"roles": {"dark": {"bg": "#201F26", "fg": "#DFDBDD"}}, "expressions": ` + c.expressions + `}`
		if err := os.WriteFile(path, []byte(js), 0o644); err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		code := Main([]string{"check", path}, "", &out, &errb)
		if _, err := theme.Load(path); err != nil {
			t.Errorf("%s: theme.Load %v, want it to load", c.expressions, err)
		}
		if c.err == "" {
			if code != 0 {
				t.Errorf("%s: check exit %d (%q), want 0", c.expressions, code, errb.String())
			}
			continue
		}
		if code != 2 || !strings.Contains(errb.String(), c.err) {
			t.Errorf("%s: check exit %d (%q), want exit 2 with %q", c.expressions, code, errb.String(), c.err)
		}
	}
}

// Eval 0's brand hexes and each kit theme's brand colours: derive's output passes check
// with nothing on stderr. derive writes the ansi16 pins check would otherwise ask for
// (a warning on error's red in both modes, for most brands), so the first check of a
// derived theme is clean, and its warning never sits beside its primary.
func TestDeriveOutputPassesCheckSilently(t *testing.T) {
	brands := [][]string{{"#0F5257", "#FF6F59"}, {"#2E8B57"}} // evals.json case 0, and the mid-tone primary it names
	kit, _ := filepath.Glob(kitThemes + "/*.json")
	if len(kit) == 0 {
		t.Fatal("no kit themes")
	}
	for _, path := range kit {
		f, err := theme.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		d, l := f.Roles["dark"], f.Roles["light"]
		brands = append(brands, []string{d.Primary, d.Secondary, "--accent", d.Accent, "--bg", d.Bg, "--light-bg", l.Bg, "--name", f.Name})
	}
	for _, args := range brands {
		out := filepath.Join(t.TempDir(), "roles.json")
		var dOut, dErr, cOut, cErr bytes.Buffer
		if code := Main(append(append([]string{"derive"}, args...), "--out", out), "", &dOut, &dErr); code != 0 {
			t.Errorf("derive %v: exit %d\n%s", args, code, dErr.String())
			continue
		}
		if strings.Contains(dErr.String(), "warning:") {
			t.Errorf("derive %v warns on its own output:\n%s", args, dErr.String())
		}
		if code := Main([]string{"check", out}, "", &cOut, &cErr); code != 0 || cErr.Len() != 0 {
			t.Errorf("check on derive %v: exit %d, stderr %q", args, code, cErr.String())
		}
		b, _ := os.ReadFile(out)
		if !strings.Contains(string(b), `"ansi16"`) && strings.Contains(dErr.String(), "pinned") {
			t.Errorf("derive %v said it pinned but wrote no ansi16 block", args)
		}
	}
}

// From one brand hue, secondary and tertiary are lightness steps of it (its hue, within
// a few degrees after gamut mapping; a step in lightness each), not a hue 40° off and a
// complement nobody chose; accent is its pale tint as before. With two hues the second
// is the brand's and the tertiary still the complement.
func TestOneHueStepsLightness(t *testing.T) {
	hueDiff := func(a, b float64) float64 {
		d := oklch.Mod(a-b, 360)
		if d > 180 {
			d = 360 - d
		}
		return d
	}
	for _, hex := range []string{"#D99A3B", "#6B50FF", "#2E8B57", "#0F5257", "#E25317", "#F5F0E1", "#0B1A10"} {
		for _, m := range Derive(hex, "", "", "", "", 0.012) {
			lp, cp, hp := oklch.LCH(m.Roles["primary"])
			for _, role := range []string{"secondary", "tertiary", "accent"} {
				l, c, h := oklch.LCH(m.Roles[role])
				if c >= 0.03 && cp >= 0.03 && hueDiff(h, hp) > 6 {
					t.Errorf("%s %s %s %s is %.0f° off primary %s: want the brand's hue", hex, m.Name, role, m.Roles[role], hueDiff(h, hp), m.Roles["primary"])
				}
				if role != "accent" && math.Abs(l-lp) < 0.05 {
					t.Errorf("%s %s %s %s is %.2f from primary %s in lightness: want a step", hex, m.Name, role, m.Roles[role], math.Abs(l-lp), m.Roles["primary"])
				}
			}
			if m.Roles["secondary"] == m.Roles["tertiary"] {
				t.Errorf("%s %s: secondary and tertiary are one colour %s", hex, m.Name, m.Roles["secondary"])
			}
		}
	}
	two := Derive("#6B50FF", "#FF60FF", "", "", "", 0.012)
	if _, _, h := oklch.LCH(two[0].Roles["secondary"]); hueDiff(h, 328) > 10 {
		t.Errorf("two hues: dark secondary %s lost the second brand hue", two[0].Roles["secondary"])
	}
}

// A warning that sits beside primary in hue and chroma (an amber brand's warning a shade
// brighter than its amber) reads as a focused row: check warns with both hexes, derive
// on that brand warns the same, and no kit theme trips it (the nearest, ferrous light,
// is 0.107 off against a 0.06 floor). Two near-greys are not "beside" each other.
func TestCheckWarnsWarningBesidePrimary(t *testing.T) {
	for _, c := range []struct {
		roles string
		want  string
	}{
		{`{"dark": {"bg": "#1A150F", "fg": "#CFC9C3", "primary": "#D99A3B", "warning": "#F2C35C"}}`,
			`dark: warning #F2C35C sits beside primary #D99A3B in hue and chroma (0.025 apart in OKLab a/b, under 0.06)`},
		{`{"dark": {"bg": "#1A150F", "fg": "#CFC9C3", "primary": "#D99A3B", "warning": "#F28C6B"}}`, ""},
		{`{"dark": {"bg": "#808080", "fg": "#FFFFFF", "primary": "#F1F2FF", "warning": "#FFFFFF"}}`, ""},
	} {
		path := filepath.Join(t.TempDir(), "theme.json")
		if err := os.WriteFile(path, []byte(`{"roles": `+c.roles+`, "ansi16": {"dark": {"primary": 11, "warning": 3}}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		Main([]string{"check", path}, "", &out, &errb)
		if got := errb.String(); strings.Contains(got, "sits beside primary") != (c.want != "") || !strings.Contains(got, c.want) {
			t.Errorf("%s: stderr %q, want %q", c.roles, got, c.want)
		}
	}
	var out, errb bytes.Buffer
	Main([]string{"derive", "#D99A3B"}, "", &out, &errb)
	if !strings.Contains(errb.String(), "warning: dark: warning #F2C35C sits beside primary #D99A3B") {
		t.Errorf("derive #D99A3B: stderr %q, want the same warning", errb.String())
	}
	kit, _ := filepath.Glob(kitThemes + "/*.json")
	for _, path := range kit {
		f, err := theme.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		for mode, r := range f.Roles {
			if d, near := NearPrimary(map[string]string{"warning": r.Warning, "primary": r.Primary}); near {
				t.Errorf("%s %s: warning %s is %.3f from primary %s", filepath.Base(path), mode, r.Warning, d, r.Primary)
			}
		}
	}
}

// A role, glyph or wordmark key the kit doesn't read is dropped by theme.Load, and the
// role it meant falls back to the derived colour with nothing to say why: check exits 2
// naming it, as it does for an expressions key.
func TestCheckRejectsUnknownKeys(t *testing.T) {
	for _, c := range []struct{ json, err string }{
		{`{"roles": {"dark": {"bg": "#201F26", "fg": "#DFDBDD", "primay": "#6B50FF"}}}`, `roles.dark.primay is not a key the kit reads (primary, secondary, tertiary, accent, bg, bgPanel, bgSubtle, bgOverlay, fg, fgSoft, fgMuted, fgSubtle, fgSelected, border, borderFocus, success, error, warning, info, onPrimary, onSecondary, onError, onWarning, onSuccess, white): a typo, or a key from a newer kit, is drawn as if unset`},
		{`{"roles": {"dark": {"bg": "#201F26", "fg": "#DFDBDD"}}, "glyphs": {"focusbar": "▌"}}`, `glyphs.focusbar is not a key the kit reads (headerRule, rule, focusBar, `},
		{`{"roles": {"dark": {"bg": "#201F26", "fg": "#DFDBDD"}}, "wordmark": {"rows": ["x"], "style": "bogus"}}`, `wordmark.style "bogus" is not a style wordmark.py writes: half | shade | ascii | tracked`},
		{`{"dark": {"bg": "#201F26", "fg": "#DFDBDD", "fgmuted": "#777777"}}`, `roles.dark.fgmuted is not a key the kit reads`},
		{`{"roles": {"dark": {"bg": "#201F26", "fg": "#DFDBDD", "white": "#FFFFFF"}}, "glyphs": {"focusBar": "▌"}, "wordmark": {"rows": ["x"], "style": "half"}}`, ""},
	} {
		path := filepath.Join(t.TempDir(), "theme.json")
		if err := os.WriteFile(path, []byte(c.json), 0o644); err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		code := Main([]string{"check", path}, "", &out, &errb)
		if _, err := theme.Load(path); err != nil {
			t.Errorf("%s: theme.Load %v, want it to load", c.json, err)
		}
		if c.err == "" {
			if code != 0 {
				t.Errorf("%s: check exit %d (%q), want 0", c.json, code, errb.String())
			}
			continue
		}
		if want := "palette: " + path + ": " + c.err; code != 2 || !strings.Contains(errb.String(), want) {
			t.Errorf("%s: check exit %d (%q), want exit 2 with %q", c.json, code, errb.String(), want)
		}
	}
}

// The engine draws every neutral on a grey at 16 colours, so only a pin puts one on a
// hue or two of the text ramp on one grey: check fails a neutral on error's red (hints
// read as errors) and warns on another hue or a shared grey, each with the grey to pin.
// (warning is pinned throughout: the kit's default warning lands on red.)
func TestCheckReportsNeutralsAt16Colours(t *testing.T) {
	roles := `{"dark": {"bg": "#17181B", "fg": "#CFCDCA", "fgMuted": "#87827D", "fgSubtle": "#615D57", "primary": "#4BC5FF"}, "light": {"bg": "#FAF9F6", "fg": "#333230", "fgMuted": "#73706D", "fgSubtle": "#9C9894", "primary": "#006C94"}}`
	for _, c := range []struct {
		name, pins string
		code       int
		stderr     []string
	}{
		{"no pins", `{"dark": {"warning": 11}, "light": {"warning": 3}}`, 0, nil},
		{"hints on red", `{"dark": {"warning": 11, "fgSubtle": 9}, "light": {"warning": 3, "fgMuted": 1}}`, 1, []string{
			`palette: ` + "%s" + `: dark: fgSubtle #615D57 draws as ANSI 9 at 16 colours (-profile ansi), error's red, so a hint reads as an error there: pin it with "ansi16": {"dark": {"fgSubtle": 8}}`,
			`light: fgMuted #73706D draws as ANSI 1 at 16 colours (-profile ansi), error's red, so secondary text reads as an error there: pin it with "ansi16": {"light": {"fgMuted": 8}}`,
			"FAIL (0 contrast problems, 2 problems at 16 colours)"}},
		{"page and text on a hue", `{"dark": {"warning": 11}, "light": {"warning": 3, "bg": 11, "fg": 4}}`, 0, []string{
			`palette: warning: ` + "%s" + `: light: bg #FAF9F6 draws as ANSI 11 at 16 colours (-profile ansi), a hue, so the page reads as a state there; "ansi16": {"light": {"bg": 15}} moves it`,
			`light: fg #333230 draws as ANSI 4 at 16 colours (-profile ansi), a hue, so body text reads as a state there; "ansi16": {"light": {"fg": 0}} moves it`}},
		{"two levels on one grey", `{"dark": {"warning": 11, "fgMuted": 8}, "light": {"warning": 3}}`, 0, []string{
			`palette: warning: ` + "%s" + `: dark: fgSubtle #615D57 draws as ANSI 8 at 16 colours (-profile ansi), as fgMuted #87827D does, so a hint reads as secondary text there; "ansi16": {"dark": {"fgSubtle": 7}} separates them`}},
		{"a pin on the right grey", `{"dark": {"warning": 11, "fgMuted": 7, "fgSubtle": 8, "bg": 0}, "light": {"warning": 3}}`, 0, nil},
	} {
		path := filepath.Join(t.TempDir(), "theme.json")
		if err := os.WriteFile(path, []byte(`{"roles": `+roles+`, "ansi16": `+c.pins+`}`), 0o644); err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		code := Main([]string{"check", path}, "", &out, &errb)
		if code != c.code {
			t.Errorf("%s: exit %d, want %d\n%s%s", c.name, code, c.code, out.String(), errb.String())
		}
		for _, want := range c.stderr {
			if strings.Contains(want, "%s") {
				want = fmt.Sprintf(want, path)
			}
			if !strings.Contains(out.String()+errb.String(), want) {
				t.Errorf("%s: output %q, want %q", c.name, errb.String()+out.String(), want)
			}
		}
		if c.stderr == nil && errb.String() != "" {
			t.Errorf("%s: stderr %q, want nothing", c.name, errb.String())
		}
	}
}

// A yellow brand sits natively on warning's slot (#FFE600 is ANSI 3 on a light page)
// while its warning lands on red: primary moves off it and warning takes its own yellow,
// rather than warning going to cyan around a primary that stays. On a dark page the
// brand is evicted to the nearest free hue; on a light one it has no hue (WarmPin16:
// cyan 6, the nearest, is 2.8:1 on white).
func TestYellowBrandKeepsWarningYellow(t *testing.T) {
	f := themeOf(Derive("#FFE600", "#556B2F", "", "", "", 0.012))
	pins := Pins16(f)
	for mode, yellow := range map[string]int{"light": 3, "dark": 11} {
		if got := pins[mode]["warning"]; got != yellow {
			t.Errorf("%s: warning pinned to %d, want its own %d (pins %v)", mode, got, yellow, pins[mode])
		}
		if p, ok := pins[mode]["primary"]; ok && p == yellow {
			t.Errorf("%s: primary pinned onto warning's %d", mode, yellow)
		}
		f.ANSI16 = pins
		if i := theme.ANSI16Index(f, mode, "primary"); i == yellow || i == 1 || i == 9 {
			t.Errorf("%s: primary draws as ANSI %d with the pins", mode, i)
		}
	}
	// check on the unpinned roles names the eviction, with the brand's own hex
	path := filepath.Join(t.TempDir(), "theme.json")
	b, _ := json.Marshal(f.Roles)
	if err := os.WriteFile(path, []byte(`{"roles": `+string(b)+`}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	Main([]string{"check", path}, "", &out, &errb)
	evicted := false
	for _, mode := range []string{"dark", "light"} {
		hex := f.Roles[mode].Primary
		native := theme.ANSI16Index(theme.File{Roles: f.Roles}, mode, "primary")
		if native == 3 || native == 11 {
			evicted = true
			want := fmt.Sprintf(`%s: primary %s draws as ANSI %d at 16 colours (-profile ansi), warning's own slot, which warning needs; "ansi16": {%q: {"primary": `, mode, hex, native, mode)
			if mode == "light" { // a yellow brand has no hue on a light page (WarmPin16): it moves to fg's grey, not to cyan 6
				want = fmt.Sprintf(`%s: primary %s (hue %.0f°, yellow) draws as ANSI %d at 16 colours (-profile ansi), yellow; on a light page a yellow brand has no slot`, mode, hex, hueOf(hex), native)
			}
			if !strings.Contains(errb.String(), want) {
				t.Errorf("%s: stderr %q, want %q", mode, errb.String(), want)
			}
		}
	}
	if !evicted {
		t.Errorf("#FFE600 sits on neither yellow natively (pins %v); the case no longer tests eviction", pins)
	}
}

// hueOf is hex's OKLCH hue.
func hueOf(hex string) float64 { _, _, h := oklch.LCH(hex); return h }

// On a mid-grey page a primary tunes to the end of the ramp, where a lightness step
// lands on the primary itself: secondary then steps in chroma, and tertiary differs from
// both (the fails case, #6B50FF on #808080).
func TestOneHueNeverCollapses(t *testing.T) {
	for _, m := range Derive("#6B50FF", "", "", "#808080", "", 0.012) {
		p, s, x := m.Roles["primary"], m.Roles["secondary"], m.Roles["tertiary"]
		if p == s || p == x || s == x {
			t.Errorf("%s: primary %s secondary %s tertiary %s: two are one colour", m.Name, p, s, x)
		}
	}
}

// A spinner whose multi-cell frames contain a status or focus mark exits 2, as a
// one-cell frame that is one did: welp's dots beside a "·" pending (#29 P7).
func TestCheckRejectsSpinnerFramesHoldingAMark(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.json")
	if err := os.WriteFile(path, []byte(`{"roles": {"dark": {"bg": "#17140F", "fg": "#E8E2D6", "primary": "#D99A3B"}}, "ansi16": {"dark": {"primary": 11, "warning": 3}}, "glyphs": {"pending": "·", "spinner": ["···", "•··", "••·", "•••"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := Main([]string{"check", path}, "", &out, &errb); code != 2 || !strings.Contains(errb.String(), `glyphs.spinner frame "···" contains glyphs.pending "·"`) {
		t.Errorf("exit %d, stderr %q", code, errb.String())
	}
}

// A dialog's roles (border, borderFocus, bgPanel, bgOverlay) are neutrals at 16 colours:
// the engine puts them on a grey, so only a pin moves one. check fails a dialog role
// pinned onto red (a ruled dialog's edge then reads as an error's) and warns on another
// hue, each with the grey that puts it back; borderFocus on primary's own slot is the
// kit's truecolour default, not a stray hue, so that pin is not reported.
func TestCheckFailsDialogRolesOnRedAt16Colours(t *testing.T) {
	roles := `{"dark": {"bg": "#17140F", "fg": "#E8E2D6", "fgMuted": "#9A9183", "fgSubtle": "#746B5A", "border": "#40372B", "borderFocus": "#746B5A", "bgOverlay": "#2E2720", "primary": "#D99A3B"}}`
	for _, c := range []struct {
		name, pins string
		code       int
		stderr     []string
	}{
		{"no pins", `{"dark": {"primary": 11, "warning": 3}}`, 0, nil},
		{"edge on red", `{"dark": {"primary": 11, "warning": 3, "borderFocus": 9}}`, 1, []string{
			`palette: ` + "%s" + `: dark: borderFocus #746B5A draws as ANSI 9 at 16 colours (-profile ansi), error's red, so the dialog's edge reads as an error there: pin it with "ansi16": {"dark": {"borderFocus": 7}}`,
			"FAIL (0 contrast problems, 1 problems at 16 colours)"}},
		{"overlay on a hue", `{"dark": {"primary": 11, "warning": 3, "bgOverlay": 4}}`, 0, []string{
			`palette: warning: ` + "%s" + `: dark: bgOverlay #2E2720 draws as ANSI 4 at 16 colours (-profile ansi), a hue, so an overlay reads as a state there; "ansi16": {"dark": {"bgOverlay": 8}} moves it`}},
		{"edge on the brand", `{"dark": {"primary": 11, "warning": 3, "borderFocus": 11}}`, 0, nil},
	} {
		path := filepath.Join(t.TempDir(), "theme.json")
		if err := os.WriteFile(path, []byte(`{"roles": `+roles+`, "ansi16": `+c.pins+`}`), 0o644); err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		code := Main([]string{"check", path}, "", &out, &errb)
		if code != c.code {
			t.Errorf("%s: exit %d, want %d\n%s%s", c.name, code, c.code, out.String(), errb.String())
		}
		for _, want := range c.stderr {
			if strings.Contains(want, "%s") {
				want = fmt.Sprintf(want, path)
			}
			if !strings.Contains(out.String()+errb.String(), want) {
				t.Errorf("%s: output %q, want %q", c.name, errb.String()+out.String(), want)
			}
		}
		if c.stderr == nil && errb.String() != "" {
			t.Errorf("%s: stderr %q, want nothing", c.name, errb.String())
		}
	}
}

// An amber brand and its warning both land on red at 16 colours. derive pins primary
// first, never to green 10 or magenta 5 with warning on the yellow (the brand amber drew
// as bright green on a dark page and magenta on a light one, beside success's green):
// on a dark page to yellow 11, with the warning on the other yellow, 3; on a light page
// to fg's grey, since yellow 3 is unreadable on white, and the warning keeps its 3 (an
// amber is warm, WarmPin16). A yellow brand that sits natively on warning's slot still
// yields it (TestYellowBrandKeepsWarningYellow).
func TestDerivePinsPrimaryToItsHueFirst(t *testing.T) {
	f := themeOf(Derive("#D99A3B", "", "", "#17140F", "#F6F1E6", 0.012))
	pins := Pins16(f)
	f.ANSI16 = pins
	for _, c := range []struct {
		mode             string
		primary, warning int
	}{{"dark", 11, 3}, {"light", 0, 3}} {
		if got := pins[c.mode]["primary"]; got != c.primary {
			t.Errorf("%s: primary pinned to %d, want %d (pins %v)", c.mode, got, c.primary, pins[c.mode])
		}
		if w := theme.ANSI16Index(f, c.mode, "warning"); w != c.warning {
			t.Errorf("%s: warning draws as ANSI %d, want yellow %d (pins %v)", c.mode, w, c.warning, pins[c.mode])
		}
	}
	// the same brand through the command: pins written, no pin for primary off yellow
	out := filepath.Join(t.TempDir(), "roles.json")
	var dOut, dErr bytes.Buffer
	if code := Main([]string{"derive", "#D99A3B", "--bg", "#17140F", "--light-bg", "#F6F1E6", "--out", out}, "", &dOut, &dErr); code != 0 {
		t.Fatalf("derive: exit %d\n%s", code, dErr.String())
	}
	if !strings.Contains(dErr.String(), "dark primary 11") || !strings.Contains(dErr.String(), "light primary 0") {
		t.Errorf("derive pinned %q", dErr.String())
	}
	if strings.Contains(dErr.String(), "of hue from its own") {
		t.Errorf("derive warns on its own pins:\n%s", dErr.String())
	}
}

// The yellow brand's eviction is per mode: a dark lime natively on 11 yields the yellow
// to its warning there, and that must not send the light amber primary, natively on
// red, back to the status-first order and onto magenta 5 (the K12 miss again: it is
// warm, so it has no hue on a light page, WarmPin16); and
// derive's own pins for the evicted yellow brand (#FFE600, light primary 0, warning 3)
// draw no far-pin warning, since the lifted theme pins primary nowhere, so there is no
// nearer pin to name (naming warning's 3 put both on one slot).
func TestEvictionIsPerMode(t *testing.T) {
	f := theme.File{Roles: map[string]theme.Roles{
		"dark":  {Bg: "#17140F", Fg: "#E8E2D6", Primary: "#C8D43A", Warning: "#F2C35C", Success: "#6ACB82", Error: "#EF8482", Info: "#56BDFF"},
		"light": {Bg: "#F6F1E6", Fg: "#2B2620", Primary: "#AF7400", Warning: "#8C6800", Success: "#007835", Error: "#A13E40", Info: "#006A9D"},
	}}
	if n := theme.ANSI16Index(f, "dark", "primary"); n != 11 {
		t.Fatalf("dark lime draws as ANSI %d natively, want 11: the probe no longer tests an eviction", n)
	}
	pins := Pins16(f)
	if got := pins["dark"]["warning"]; got != 11 {
		t.Errorf("dark: warning pinned to %d, want its own 11 (the lime yields it)", got)
	}
	if got := pins["light"]["primary"]; got != 0 {
		t.Errorf("light: primary pinned to %d, want fg's grey 0, a warm brand's on a light page (pins %v)", got, pins["light"])
	}
	yellow := themeOf(Derive("#FFE600", "#556B2F", "", "", "", 0.012))
	yellow.ANSI16 = Pins16(yellow)
	if far := FarPins(yellow); len(far) != 0 {
		t.Errorf("derive's own pins for the yellow brand warn: %+v", far)
	}
	// through the command too: derive's stderr and check's on its output carry no far
	// pin (the brand's warning beside its yellow is a warning on purpose, so this brand
	// is not in TestDeriveOutputPassesCheckSilently)
	out := filepath.Join(t.TempDir(), "roles.json")
	var dOut, dErr, cOut, cErr bytes.Buffer
	if code := Main([]string{"derive", "#FFE600", "#556B2F", "--out", out}, "", &dOut, &dErr); code != 0 {
		t.Fatalf("derive: exit %d\n%s", code, dErr.String())
	}
	Main([]string{"check", out}, "", &cOut, &cErr)
	for what, text := range map[string]string{"derive": dErr.String(), "check": cErr.String()} {
		if strings.Contains(text, "of hue from its own") {
			t.Errorf("%s warns on derive's own pins for the yellow brand:\n%s", what, text)
		}
	}
}

// A pin that moves a role far from its own hue (a rose primary on blue 12, 109° off)
// while the tool itself would pin it nearer reads as another state: check warns, naming
// the hex, its hue, the slot and the nearer pin. A pin under 60° (the rose on magenta
// 13, 45° off) is not reported; nor is a neutral's. A warm brand off its slot (the
// amber on green 10, blue 12 or magenta 5) is Findings16's, which names the slot
// (WarmPin16), not a far pin: its slot is no nearer its hue than magenta, so "nearer"
// would be false (#51).
func TestCheckWarnsOnAFarPin(t *testing.T) {
	rose := `{"dark": {"bg": "#17140F", "fg": "#E8E2D6", "primary": "#D94060", "warning": "#F2C35C", "success": "#6ACB82", "error": "#EF8482", "info": "#56BDFF"}}`
	amber := `{"dark": {"bg": "#17140F", "fg": "#E8E2D6", "primary": "#D99A3B", "warning": "#F2C35C", "success": "#6ACB82", "error": "#EF8482", "info": "#56BDFF"}, "light": {"bg": "#F6F1E6", "fg": "#2B2620", "primary": "#AF7400", "warning": "#8C6800", "success": "#007835", "error": "#A13E40", "info": "#006A9D"}}`
	for _, c := range []struct {
		name, roles, pins string
		want, not         []string
	}{
		{"a hand pin far off", rose, `{"dark": {"primary": 12, "warning": 11}}`, []string{
			`dark: primary #D94060 is pinned to ANSI 12 at 16 colours (-profile ansi), bright blue, 109° of hue from its own 13°, so the brand reads as blue there; "ansi16": {"dark": {"primary": 13}} is nearer its hue (magenta, 45° off)`,
		}, nil},
		{"with the nearer slot held", rose, `{"dark": {"primary": 12, "warning": 11, "info": 13}}`, []string{
			`"ansi16": {"dark": {"primary": 13}} is nearer its hue (magenta, 45° off), with info moved off it`,
		}, nil},
		{"the nearest free hue", rose, `{"dark": {"primary": 13, "warning": 11}}`, nil, []string{"of hue from its own"}},
		{"derive's old pins for an amber", amber, `{"dark": {"primary": 10, "warning": 11}, "light": {"primary": 5, "warning": 3}}`, []string{
			`dark: primary #D99A3B (hue 74°, warm) draws as ANSI 10 at 16 colours (-profile ansi), bright green; a warm brand has no slot of its own there`, `"ansi16": {"dark": {"primary": 11}} moves it`,
			`light: primary #AF7400 (hue 74°, warm) draws as ANSI 5 at 16 colours (-profile ansi), magenta; on a light page a warm brand has no slot`, `"ansi16": {"light": {"primary": 0}} moves it`,
		}, []string{"of hue from its own"}},
		{"an amber on its slots", amber, `{"dark": {"primary": 11, "warning": 3}, "light": {"primary": 0}}`, nil, []string{"of hue from its own", "warm"}},
	} {
		path := filepath.Join(t.TempDir(), "theme.json")
		if err := os.WriteFile(path, []byte(`{"roles": `+c.roles+`, "ansi16": `+c.pins+`}`), 0o644); err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		Main([]string{"check", path}, "", &out, &errb)
		for _, want := range c.want {
			if !strings.Contains(errb.String(), want) {
				t.Errorf("%s: stderr %q, want %q", c.name, errb.String(), want)
			}
		}
		for _, not := range c.not {
			if strings.Contains(errb.String(), not) {
				t.Errorf("%s: stderr %q, want no %q", c.name, errb.String(), not)
			}
		}
	}
	// no kit theme's own pins are far: each is the nearest free hue, or under 60°
	kit, _ := filepath.Glob(kitThemes + "/*.json")
	for _, path := range append(kit, kitThemes+"/../theme/testdata/quire.json") {
		var out, errb bytes.Buffer
		Main([]string{"check", path}, "", &out, &errb)
		if strings.Contains(errb.String(), "of hue from its own") {
			t.Errorf("%s: %s", filepath.Base(path), errb.String())
		}
	}
}

// The border floor matches what a reader can see: check fails a border under 1.3:1 on
// its page and warns under 1.5:1 (welp's #2A231A on #17140F, 1.18:1, passed the old 1.14
// floor with every rule invisible on the sheet); 1.9 stays the ceiling. derive puts its
// border at 1.5–1.9 on any page, so its output never warns; light pages get the same
// numbers, since a 1.5:1 step is about 15 L* on a dark page and on a light one alike.
func TestBorderFloor(t *testing.T) {
	for _, c := range []struct {
		name, border string
		code         int
		stdout       string
		stderr       []string
	}{
		{"invisible", "#2A231A", 1, "FAIL border       on bg         1.18  (want ≥1.3 ≤1.9)", []string{"FAIL (1 contrast problems)"}},
		{"faint", "#383026", 0, "warn border       on bg         1.42  (want ≥1.3 ≤1.9)", []string{
			`palette: warning: ` + "%s" + `: dark: border #383026 on bg #17140F is 1.42:1, under 1.5: the section, table and footer rules are barely there; a rule reads at 1.5–1.9`}},
		{"visible", "#40372B", 0, "ok   border       on bg         1.57  (want ≥1.3 ≤1.9)", nil},
	} {
		path := filepath.Join(t.TempDir(), "theme.json")
		if err := os.WriteFile(path, []byte(`{"roles": {"dark": {"bg": "#17140F", "fg": "#E8E2D6", "border": "`+c.border+`", "primary": "#D99A3B"}}, "ansi16": {"dark": {"primary": 11, "warning": 3}}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		code := Main([]string{"check", path}, "", &out, &errb)
		if code != c.code {
			t.Errorf("%s: exit %d, want %d\n%s%s", c.name, code, c.code, out.String(), errb.String())
		}
		if !strings.Contains(out.String(), c.stdout) {
			t.Errorf("%s: stdout %q, want %q", c.name, out.String(), c.stdout)
		}
		for _, want := range c.stderr {
			if strings.Contains(want, "%s") {
				want = fmt.Sprintf(want, path)
			}
			if !strings.Contains(out.String()+errb.String(), want) {
				t.Errorf("%s: output %q, want %q", c.name, errb.String()+out.String(), want)
			}
		}
		if c.stderr == nil && errb.String() != "" {
			t.Errorf("%s: stderr %q, want nothing", c.name, errb.String())
		}
	}
	for _, args := range [][]string{{"#D99A3B", "", "", "#17140F", "#F6F1E6"}, {"#6B50FF", "", "", "", ""}, {"#6B50FF", "", "", "#808080", ""}, {"#0F5257", "#FF6F59", "", "", ""}, {"#F5F0E1", "", "", "", ""}} {
		for _, m := range Derive(args[0], args[1], args[2], args[3], args[4], 0.012) {
			if cr := oklch.Contrast(m.Roles["border"], m.Roles["bg"]); cr < 1.5 || cr > 1.9 {
				t.Errorf("derive %v %s: border %s on bg %s is %.2f:1, want 1.5–1.9", args, m.Name, m.Roles["border"], m.Roles["bg"], cr)
			}
		}
	}
}
