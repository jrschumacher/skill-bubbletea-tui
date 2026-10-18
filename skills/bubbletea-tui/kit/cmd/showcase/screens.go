package main

import (
	"strings"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/ultraviolet/layout"

	"tuikit/theme"
	"tuikit/ui"
)

const (
	minW, minH = 60, 18
	wideW      = 110 // detail pane / sidebar appear at this width
)

// render draws the whole screen for the current state at w×h. The live
// View and the golden harness share it, so what the PNG shows is what the
// terminal shows.
func (m *app) render(w, h int) *ui.Canvas {
	m.w, m.h = w, h
	c := m.draw(w, h)
	if m.t.ASCII {
		c.ASCIIfy() // nothing outside tier 0 survives, whatever a screen typed
	}
	return c
}

func (m *app) draw(w, h int) *ui.Canvas {
	t := m.t
	c := ui.NewCanvas(t, w, h)
	if w < minW || h < minH {
		c.Draw(ui.TooSmall(t, w, h, minW, minH, m.footerBindings()), c.Bounds())
		return c
	}
	if m.screen == scrSplash {
		m.splash(c)
		return c
	}
	g := t.X.Gutter
	s := m.svcs[m.sel]
	details := ui.Details(t, "prod", "us-east-1", s.Name)
	if m.screen == scrSetup {
		details = ui.Details(t, "setup", m.setup.steps(t).Count())
	}
	head := ui.Header(t, w, h, t.AppName(), details)
	y := 0
	c.Draw(strings.Join(head, "\n"), ui.Rect(0, y, w, len(head)))
	y += len(head) + t.Gap()

	// footer first: the body gets what is left
	foot := ui.Footer(t, w, m.footerBadge(), m.footerBindings(), m.footerRight())
	fy := h - len(foot)
	c.Draw(strings.Join(foot, "\n"), ui.Rect(0, fy, w, len(foot)))

	if m.screen == scrSetup {
		// the wizard's step row stands where the tabs stand
		c.Draw(m.setup.steps(t).Row(t, w-2*g), ui.Rect(g, y, w-2*g, 1))
		y += 2
	} else if m.screen != scrDeploy && m.screen != scrLadder {
		counts := []string{t.S.Subtle.Render(itoa(len(m.svcs))), "", "", ""}
		if m.declared {
			counts[2] = t.S.Warn.Render("1")
		}
		tabs := ui.Tabs(t, tabNames, int(m.screen), counts)
		c.Draw(strings.Join(tabs, "\n"), ui.Rect(g, y, w-2*g, len(tabs)))
		y += len(tabs) + 1 // tabs always get air below, even in compact brands
	}
	body := uv.Rect(g, y, w-2*g, max(0, fy-y-t.Gap()))
	m.body, m.footY = body, fy
	switch m.screen {
	case scrServices:
		m.services(c, body)
	case scrDeploys:
		m.deploys(c, body)
	case scrIncidents:
		m.incidents(c, body)
	case scrAssistant:
		m.assistant(c, body)
	case scrDeploy:
		m.deployForm(c, body)
	case scrLadder:
		m.ladder(c, body)
	case scrSetup:
		m.setupView(c, body)
	}
	// a dialog takes the rows it needs between the tabs and the footer (dialogTop keeps
	// a row of air above the footer while it fits) and scrolls its body past that
	switch m.ov {
	case ovPalette:
		m.palette(c, fy)
	case ovConfirm:
		m.confirm(c, fy)
	case ovHelp:
		m.keysOverlay(c, fy)
	}
	if m.ov != ovNone {
		// the footer now describes the dialog's keys: it sits above the scrim
		c.Draw(strings.Join(foot, "\n"), ui.Rect(0, fy, w, len(foot)))
	}
	if m.toast != nil {
		c.Draw(ui.Toast(t, m.toast.kind, m.toast.label, m.toast.msg, w), ui.Rect(0, h-1, w, 1))
	}
	return c
}

func (m *app) footerBadge() string {
	if m.screen == scrDeploy && m.ov == ovNone {
		return ui.Tag(m.t, "warning", "PROD")
	}
	if m.screen == scrSetup && m.ov == ovNone {
		return ui.Tag(m.t, "info", "SETUP")
	}
	return ""
}

func (m *app) footerRight() string {
	if m.screen == scrServices && m.ov == ovNone {
		if m.paused {
			return m.t.S.Subtle.Render("paused")
		}
		return m.t.S.Subtle.Render("synced 4s ago")
	}
	if m.screen == scrSetup && m.ov == ovNone && !m.setup.saved {
		return m.t.S.Subtle.Render("not saved yet")
	}
	return ""
}

// ---------------------------------------------------------------- services

func (m *app) services(c *ui.Canvas, r uv.Rectangle) {
	t := m.t
	var main, side uv.Rectangle
	if r.Dx() >= wideW-4 {
		layout.Horizontal(layout.Fill(1), layout.Len(3), layout.Len(40)).Split(r).Assign(&main, new(uv.Rectangle), &side)
	} else {
		main = r
	}
	wide := main.Dx() >= 70
	header := []string{"service", "version", "p99", "errors"}
	widths := []int{0, 8, 12, 6}
	align := "llll"
	if wide {
		header = append(header, "deployed")
		widths = []int{0, 8, 12, 6, 8}
		align = "llllr"
	}
	fixed := 2
	for _, x := range widths[1:] {
		fixed += x + 2
	}
	widths[0] = max(12, main.Dx()-fixed)
	var rows [][]string
	for i, s := range m.svcs {
		name := ui.Dot(t, s.State) + " " + t.S.Base.Render(s.Name)
		if s.State == "deploying" {
			name = ui.Busy(t, m.frame/2) + " " + t.S.Base.Render(s.Name)
		}
		errs := t.S.Soft.Render(s.ErrRate)
		if s.State == "error" {
			errs = t.S.Err.Bold(true).Render(s.ErrRate)
		}
		ver := t.S.Muted.Render(s.Version)
		if i == m.sel {
			ver = t.S.Soft.Render(s.Version)
		}
		row := []string{name, ver, ui.StateSparkline(t, s.State, s.P99, 12), errs}
		if wide {
			row = append(row, t.S.Subtle.Render(s.Deployed))
		}
		rows = append(rows, row)
	}
	tbl := ui.Table(t, header, rows, widths, align, m.sel, main.Dx(), m.ov == ovNone)
	c.Draw(strings.Join(tbl, "\n"), main)
	s := m.svcs[m.sel]
	if side.Dx() > 0 {
		m.serviceDetail(c, side)
		// tall terminals: use the room for the activity feed, not blank rows
		if top := main.Min.Y + len(tbl) + 2; main.Max.Y-top >= 6 {
			act := []string{ui.Section(t, "Activity", main.Dx(), "last hour"), ""}
			for _, e := range m.feed {
				act = append(act, "  "+t.S.Subtle.Render(e[0])+"  "+ui.Dot(t, e[3])+" "+t.S.Soft.Render(e[1])+"  "+t.S.Muted.Render(e[2]))
			}
			n := min(len(act), main.Max.Y-top)
			c.Draw(strings.Join(act[:n], "\n"), uv.Rect(main.Min.X, top, main.Dx(), n))
		}
	} else if y := main.Min.Y + len(tbl) + 1; y < main.Max.Y {
		line := t.S.Muted.Render(s.Name) + t.S.Subtle.Render("  owner ") + t.S.Soft.Render(s.Owner) +
			t.S.Subtle.Render("  deployed ") + t.S.Soft.Render(s.Deployed)
		c.Draw(line, uv.Rect(main.Min.X+2, y, main.Dx()-2, 1))
		if s.State == "error" && y+2 < main.Max.Y {
			c.Draw(ui.Alert(t, "error", "502s", "4.8% of checkout requests failing since 14:02 "+t.G.Sep+" r roll back", main.Dx()-2), uv.Rect(main.Min.X+2, y+2, main.Dx()-2, 1))
		}
	}
}

var stateTag = map[string][2]string{
	"error": {"error", "FAILING"}, "ok": {"ok", "HEALTHY"}, "warning": {"warning", "DEGRADED"},
	"deploying": {"primary", "DEPLOYING"}, "idle": {"muted", "IDLE"},
}

func (m *app) serviceDetail(c *ui.Canvas, r uv.Rectangle) {
	t := m.t
	s := m.svcs[m.sel]
	w := r.Dx()
	tag := stateTag[s.State]
	rows := []string{ui.Section(t, s.Name, w, s.Env), ""}
	rows = append(rows, ui.KV(t, [][2]string{
		{"status", ui.Tag(t, tag[0], tag[1])},
		{"version", t.S.Base.Render(s.Version)},
		{"owner", t.S.Base.Render(s.Owner)},
		{"deployed", t.S.Base.Render(s.Deployed)},
	}, 9)...)
	rows = append(rows, "", ui.Section(t, "p99 latency", w, "12 min"), "")
	rows = append(rows, ui.Sparkline(t, stretch(s.P99, w), w, t.S.Muted))
	last := s.P99[len(s.P99)-1]
	trend := t.S.Base.Render(itoa(int(last)) + "ms")
	if s.P99[0] > 0 && last/s.P99[0] > 2 {
		trend += "  " + t.S.Err.Render(t.G.Arrow+" "+ftoa(last/s.P99[0])+"x since 14:00")
	}
	rows = append(rows, trend)
	// Recent: this service's lines from the live feed
	var recent []string
	for _, e := range m.feed {
		if strings.Contains(e[1], s.Name) && len(recent) < 3 {
			recent = append(recent, t.S.Subtle.Render(e[0])+"  "+t.S.Soft.Render(ui.Trunc(t, e[1], w-7)))
		}
	}
	if len(recent) > 0 {
		rows = append(rows, "", ui.Section(t, "Recent", w, ""), "")
		rows = append(rows, recent...)
	}
	if s.State == "error" && s.Prev != "" {
		rows = append(rows, "", t.S.Key.Render("r")+" "+t.S.KeyDesc.Render("roll back to "+s.Prev))
	}
	c.Draw(strings.Join(rows, "\n"), r)
}

// stretch resamples vals to n points (nearest), so a sparkline fills its column.
func stretch(vals []float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = vals[i*len(vals)/n]
	}
	return out
}

// ---------------------------------------------------------------- deploys

func (m *app) deploys(c *ui.Canvas, r uv.Rectangle) {
	t := m.t
	var main, side uv.Rectangle
	if r.Dx() >= wideW-4 {
		layout.Horizontal(layout.Fill(1), layout.Len(3), layout.Len(40)).Split(r).Assign(&main, new(uv.Rectangle), &side)
		m.serviceDetail(c, side) // the selected row's service: the same one every action uses
	} else {
		main = r
	}
	var rows [][]string
	for _, e := range m.events {
		st := ui.Dot(t, e.State)
		if e.State == "busy" {
			st = ui.Busy(t, m.frame/2)
		}
		name := t.S.Base.Render(e.Svc)
		if e.Env != "prod" {
			name += t.S.Muted.Render(" " + e.Env)
		}
		rows = append(rows, []string{st + " " + name, t.S.Soft.Render(e.Version), t.S.Muted.Render(e.By), t.S.Subtle.Render(e.Ago)})
	}
	widths := []int{0, 9, 8, 8}
	widths[0] = max(12, min(40, main.Dx()-(2+11+10+10)))
	tbl := ui.Table(t, []string{"service", "version", "by", "when"}, rows, widths, "lllr", m.depSel, main.Dx(), m.ov == ovNone)
	c.Draw(strings.Join(tbl, "\n"), main)
}

// ---------------------------------------------------------------- incidents

func (m *app) incidents(c *ui.Canvas, r uv.Rectangle) {
	t := m.t
	act := t.S.Key.Render("n") + " " + t.S.KeyDesc.Render("declare incident") + "   " + t.S.Key.Render("h") + " " + t.S.KeyDesc.Render("history")
	title, why := "No incidents declared", "checkout-api has been failing for 12 min. Declare one to page payments on-call."
	if m.declared {
		title, why, act = "INC-2041 declared", "Paging payments on-call. Updates will appear here as responders join.", ""
	}
	rows := ui.Empty(t, r.Dx(), "", title, why, act)
	top := r.Min.Y + max(0, (r.Dy()-len(rows))/2-1)
	c.Draw(strings.Join(rows, "\n"), uv.Rect(r.Min.X, top, r.Dx(), len(rows)))
}

// ---------------------------------------------------------------- ladder

// ladder is the status legend: every rung of the theme's ladder in its role, the
// spinner, a step row and the tags, so a brand's marks are judged from one frame in
// every profile (brand.md §6).
func (m *app) ladder(c *ui.Canvas, r uv.Rectangle) {
	t := m.t
	w := r.Dx()
	rows := []string{ui.Section(t, "Status", w, strings.Join([]string{"glyph", "meaning", "slot", "role"}, " "+t.G.Sep+" ")), ""}
	rows = append(rows, ui.Ladder(t, m.frame/2)...)
	if r.Dy() >= len(rows)+7 {
		steps := ui.Steps{Names: []string{"Build", "Test", "Deploy", "Verify"}, Cur: 2}
		rows = append(rows, "", ui.Section(t, "Steps", w, ""), "", steps.Row(t, w))
		tags := []string{ui.Tag(t, "ok", "HEALTHY"), ui.Tag(t, "warning", "DEGRADED"), ui.Tag(t, "error", "FAILING"),
			ui.Tag(t, "info", "SETUP"), ui.Tag(t, "primary", "NEW"), ui.Tag(t, "muted", "IDLE")}
		rows = append(rows, "", ui.Section(t, "Tags", w, ""), "", strings.Join(tags, " "))
	}
	// text beside the marks: a reading with its unit, a product, a name with an accent.
	// Text is not a glyph (C-GLYPH passes it); under -ascii it keeps its letters and
	// digits (21.5 C, 3x4, Zoe), never a "?". Every size shows it, so the ASCII test sees it.
	if r.Dy() >= len(rows)+2 {
		rows = append(rows, "")
	}
	if r.Dy() >= len(rows)+1 {
		rows = append(rows, t.S.Muted.Render("text")+"  "+t.S.Base.Render("21.5°C")+t.S.Subtle.Render(" "+t.G.Sep+" ")+
			t.S.Base.Render("3×4")+t.S.Subtle.Render(" "+t.G.Sep+" ")+t.S.Base.Render("Zoë"))
	}
	c.Draw(strings.Join(rows, "\n"), r)
}

// ---------------------------------------------------------------- assistant

func (m *app) assistant(c *ui.Canvas, r uv.Rectangle) {
	t := m.t
	var main, side uv.Rectangle
	if r.Dx() >= wideW-4 {
		layout.Horizontal(layout.Fill(1), layout.Len(3), layout.Len(30)).Split(r).Assign(&main, new(uv.Rectangle), &side)
		m.sessionSidebar(c, side)
	} else {
		main = r
	}
	w := main.Dx()
	diff := make([]string, len(diffLines))
	for i, l := range diffLines {
		switch {
		case strings.HasPrefix(l, "+"):
			diff[i] = t.S.OK.Render(l)
		case strings.HasPrefix(l, "-"):
			diff[i] = t.S.Err.Render(l)
		default: // hunk headers sit on the panel colour: muted, not subtle
			diff[i] = t.S.Muted.Render(l)
		}
	}
	shown := 4
	if m.expand {
		shown = len(logLines)
	}
	// the stream is blocks (a turn, a tool call, a thinking panel), so the tail cut can
	// keep every block's head on screen (ui.Transcript)
	blocks := [][]string{
		ui.UserMessage(t, chatUser, w, false),
		ui.Thinking(t, thinkingLines, 2, "6.1s", w),
		ui.ToolCall(t, ui.Tool{Name: "Logs", Param: "checkout-api", Opts: [][2]string{{"since", "14:00"}, {"tail", "200"}},
			Meta: "1.2s", State: ui.ToolDone, Body: logLines, Shown: shown, Expand: m.expand}, w, false),
		ui.ToolCall(t, ui.Tool{Name: "Grep", Param: "PAYMENTS_TIMEOUT", Opts: [][2]string{{"path", "deploy/"}, {"ignore case", ""}},
			State: ui.ToolError, Err: "deploy/: no such directory (did you mean config/?)"}, w, false),
		ui.ToolCall(t, ui.Tool{Name: "Edit", Param: "config/timeouts.go", Desc: "Accept a unit suffix on PAYMENTS_TIMEOUT",
			Meta: "allowed for session " + t.G.Sep + " 0.3s", State: ui.ToolDone, Body: diff, Expand: true}, w, false),
		ui.ToolCall(t, ui.Tool{Name: "Bash", Param: "go test ./internal/...", State: ui.ToolDone, Body: testLines}, w, false),
		ui.AssistantMessage(t, chatAnswer, w, false),
		{ui.TurnFooter(t, "opus", "anthropic", "41s", w)},
	}
	for _, msg := range m.sent {
		switch msg {
		case "\x00done":
			blocks = append(blocks, ui.ToolCall(t, ui.Tool{Name: "Deploy", Param: "checkout-api v2.4.1", Opts: [][2]string{{"strategy", "rolling"}},
				State: ui.ToolDone, Body: []string{"3/3 pods healthy " + t.G.Sep + " error rate 0.02% " + t.G.Sep + " p99 124ms"}}, w, false))
		case "\x00canceled":
			blocks = append(blocks, []string{"  " + t.S.Subtle.Render(t.Sentence("Canceled."))})
		default:
			blocks = append(blocks, ui.UserMessage(t, msg, w, false))
		}
	}
	editorH := 3
	streamH := main.Dy() - editorH - 1
	if m.busy {
		blocks = append(blocks, []string{"  " + ui.NewAnim(t, 10, "Deploying checkout-api v2.4.1", "deploy").Frame(m.frame, elapsed(m.frame-m.busySince))})
	}
	// follow mode: the tail of the stream, minus how far the user scrolled up
	s := ui.Transcript(t, blocks, w, streamH, m.scroll, t.S.Key.Render("pgup"))
	c.Draw(strings.Join(s, "\n"), uv.Rect(main.Min.X, main.Min.Y, w, streamH))
	ey := main.Max.Y - editorH
	prompt := t.S.Prompt.Bold(true).Render(t.G.Prompt) + " "
	text := m.input
	if m.ov != ovNone { // under a dialog the composer is unfocused: a quiet prompt
		prompt = t.S.Subtle.Render(t.G.Prompt) + " "
	}
	if m.busy {
		prompt = t.S.Subtle.Render(t.G.Prompt) + " "
		text = ""
	}
	// under a dialog the composer has no caret (the dialog has the focus); a blank
	// keeps the placeholder where it was
	caret := t.S.Prompt.Render(t.G.Caret)
	if m.ov != ovNone {
		caret = strings.Repeat(" ", lipgloss.Width(t.G.Caret))
	}
	line := prompt + t.S.Base.Render(text)
	switch {
	case m.busy:
		line += t.S.Subtle.Render("working" + t.G.Ellipsis + " esc to stop")
	case text == "":
		line += caret + t.S.Subtle.Render("ask, or tell it what to do")
	default:
		line += caret
	}
	c.Draw(strings.Join([]string{ui.Rule(t, w), line}, "\n"), uv.Rect(main.Min.X, ey, w, editorH))
}

func elapsed(frames int) string {
	s := frames * int(ui.FrameInterval.Milliseconds()) / 1000
	return itoa(s) + "s"
}

func (m *app) sessionSidebar(c *ui.Canvas, r uv.Rectangle) {
	t := m.t
	w := r.Dx()
	rows := []string{
		t.S.Bold.Render("checkout 502s"),
		t.S.Subtle.Render("started 14:06 " + t.G.Sep + " " + itoa(3+len(m.sent)) + " turns"),
		"",
		ui.Section(t, "Model", w, ""),
		t.S.Muted.Render(t.G.Todo+" ") + t.S.Base.Render("opus") + t.S.Subtle.Render(" via anthropic"),
		t.S.Subtle.Render("context ") + ui.Progress(t, 0.42, 10) + t.S.Subtle.Render(" 42%"),
		"",
		ui.Section(t, "Changed", w, "1"),
		t.S.Soft.Render("config/timeouts.go ") + t.S.OK.Render("+4") + " " + t.S.Err.Render("-1"),
		"",
		ui.Section(t, "Touched", w, ""),
	}
	for _, s := range m.svcs[:4] {
		rows = append(rows, ui.Dot(t, s.State)+" "+t.S.Soft.Render(ui.Trunc(t, s.Name, w-2)))
	}
	c.Draw(strings.Join(rows, "\n"), r)
}

// ---------------------------------------------------------------- deploy form

func (m *app) deployForm(c *ui.Canvas, r uv.Rectangle) {
	t := m.t
	s := m.svcs[m.sel]
	var main, side uv.Rectangle
	if r.Dx() >= wideW-4 {
		layout.Horizontal(layout.Len(min(64, r.Dx()-44)), layout.Len(4), layout.Fill(1)).Split(r).Assign(&main, new(uv.Rectangle), &side)
	} else {
		main = r
	}
	w := main.Dx()
	rows := []string{ui.Section(t, "Deploy "+s.Name, w, s.Env), ""}
	// under a dialog no field is focused: the dialog has the focus
	focus := m.focus
	if m.ov != ovNone {
		focus = -1
	}
	// a field is a group: one blank row between fields even in compact brands
	field := func(i int, label, hint string, body ...string) {
		focused := focus == i
		bar := ui.Gutter(t, focused, t.S.FocusBar)
		lab := t.S.Muted.Render(t.Case(label))
		if focused {
			lab = t.S.Bold.Render(t.Case(label))
		}
		rows = append(rows, bar+lab)
		if hint != "" {
			rows = append(rows, bar+t.S.Subtle.Render(hint))
		}
		for _, b := range body {
			rows = append(rows, bar+b)
		}
		rows = append(rows, "")
	}
	choice := func(opts []string, on int, focused bool) string {
		var out []string
		for i, o := range opts {
			switch {
			case i == on && focused:
				out = append(out, t.S.Focus.Render(t.G.RadioOn)+" "+t.S.Bold.Render(o))
			case i == on:
				out = append(out, t.S.Muted.Render(t.G.RadioOn)+" "+t.S.Base.Render(o))
			default:
				out = append(out, t.S.Subtle.Render(t.G.RadioOff)+" "+t.S.Muted.Render(o))
			}
		}
		return strings.Join(out, "   ")
	}
	field(0, "Environment", "", choice([]string{"prod", "staging", "dev"}, m.env, focus == 0))
	promptSt := t.S.Subtle // a blurred input's prompt is quiet: the focus ink means focus
	if focus == 1 {
		promptSt = t.S.Prompt
	}
	input := promptSt.Render(t.G.Prompt) + " " + t.S.Base.Render(m.version)
	if focus == 1 {
		input += t.S.Prompt.Render(t.G.Caret)
	}
	field(1, "Version", "tag or commit; latest is v2.4.1 (2 min ago)", input)
	strategies := [][2]string{{"Rolling", "one pod at a time"}, {"Canary", "10% for 15 min, then all"}, {"Blue/green", "switch traffic at once"}}
	field(2, "Strategy", strategies[m.strategy][1], choice([]string{"Rolling", "Canary", "Blue/green"}, m.strategy, focus == 2))
	check := func(i int, label string) string {
		g, st := t.G.CheckOff, t.S.Subtle
		if m.checks[i] {
			g, st = t.G.CheckOn, t.S.Focus
		}
		lab := t.S.Muted.Render(label)
		if focus == 3 && m.checkSel == i {
			lab = t.S.Bold.Render(label)
		}
		return st.Render(g) + " " + lab
	}
	field(3, "Checks", "", check(0, "run smoke tests")+"   "+check(1, "skip approval"))
	btns := ui.Buttons(t, w-2, lipgloss.Left,
		ui.Button(t, "Deploy", focus == 4 && m.formBtn == 0, false, -1),
		ui.Button(t, "Cancel", focus == 4 && m.formBtn == 1, false, -1))
	rows = append(rows, ui.Gutter(t, focus == 4, t.S.FocusBar)+btns)
	c.Draw(strings.Join(rows, "\n"), main)

	if side.Dx() > 0 {
		sw := side.Dx()
		// each step's detail on its own line under the tree's rail
		steps := []string{t.S.Soft.Render("build image") + "\n" + t.S.Subtle.Render("cached")}
		if m.checks[0] {
			steps = append(steps, t.S.Soft.Render("run smoke tests")+"\n"+t.S.Subtle.Render("~40s"))
		}
		steps = append(steps, t.S.Soft.Render([]string{"roll 3 pods", "canary 1 pod", "start 3 new pods"}[m.strategy])+"\n"+t.S.Subtle.Render(strategies[m.strategy][1]),
			t.S.Soft.Render("watch error rate")+"\n"+t.S.Subtle.Render("5 min, auto-rollback >1%"))
		pr := []string{ui.Section(t, "What happens", sw, ""), ""}
		pr = append(pr, t.S.Base.Render(s.Name+" ")+t.S.Muted.Render(s.Version+" "+t.G.Arrow+" ")+t.S.Primary.Bold(true).Render(m.version))
		pr = append(pr, "")
		pr = append(pr, ui.Tree(t, steps)...)
		if m.env == 0 {
			pr = append(pr, "", ui.Alert(t, "warning", "PROD", "customer traffic, 3 regions", sw))
		}
		c.Draw(strings.Join(pr, "\n"), side)
	}
}

// ---------------------------------------------------------------- overlays

type hit struct {
	c   command
	idx []int
}

func (m *app) filtered() []hit {
	var out []hit
	q := strings.ToLower(m.query)
	for _, c := range commands {
		if idx, ok := ui.Fuzzy(strings.ToLower(c.Title), q); ok {
			out = append(out, hit{c, idx})
		}
	}
	return out
}

// dialogRoom is the rows a dialog may take on a screen whose footer starts at fy: from
// row 3 (under the header) to the row above the footer. A dialog that needs fewer sits a
// row lower with a row of air above the footer (dialogTop); one that needs more scrolls.
func dialogRoom(fy int) int { return fy - 3 }

// dialogTop keeps a dialog below the header and tabs (row 4, or 3 when it needs the
// row) and above the footer, which starts at fy.
func dialogTop(c *ui.Canvas, dh, fy int) *uv.Position {
	h := c.Bounds().Dy()
	y := max(4, (h-dh)/2-1)
	y = min(y, max(0, fy-dh))
	return &uv.Position{X: -1, Y: y} // X < 0: centre horizontally
}

func (m *app) palette(c *ui.Canvas, fy int) {
	room := dialogRoom(fy)
	t := m.t
	w := min(64, c.Bounds().Dx()-6)
	inner := w - t.S.Dialog.GetHorizontalFrameSize()
	hits := m.filtered()
	var items []ui.Item
	for _, h := range hits {
		items = append(items, ui.Item{Title: h.c.Title, Desc: h.c.Desc, Right: h.c.Key, Match: h.idx})
	}
	input := t.S.Prompt.Render(t.G.Prompt) + " " + t.S.Base.Render(m.query) + t.S.Prompt.Render(t.G.Caret)
	if m.query == "" {
		input = t.S.Prompt.Render(t.G.Prompt) + " " + t.S.Prompt.Render(t.G.Caret) + t.S.Subtle.Render("type to filter")
	}
	// the list fits its results (min 3) and the room the dialog has (the input row and
	// its gap taken off); the top edge stays put while typing
	listH := min(max(3, len(hits)), len(commands), max(3, ui.DialogBodyRows(t, room, "Commands", "", "")-2))
	body := []string{input, ""}
	if len(items) == 0 {
		body = append(body, t.S.Muted.Render("  no command matches \""+m.query+"\""))
		for range listH - 1 {
			body = append(body, "")
		}
	} else {
		sel := min(m.palSel, len(items)-1)
		m.palOff = ui.Follow(sel, m.palOff, listH, len(items)) // the list scrolls to the selection
		body = append(body, ui.List(t, items, sel, m.palOff, inner, listH, true)...)
	}
	// no help line inside: the footer above the scrim carries the dialog's keys
	d := ui.Popover(t, w, room, "Commands", body, "", "")
	m.ovRect = c.Popover(d, &uv.Position{X: -1, Y: min(4, max(0, fy-lipgloss.Height(d)))})
}

// confirmWidth is the confirm dialog's outer width at the model's size.
func (m *app) confirmWidth() int { return min(58, m.w-6) }

// confirmBody is the confirm dialog's title, body rows and button row.
func (m *app) confirmBody() (title string, body []string, btns string) {
	t := m.t
	s := m.svcs[m.sel]
	inner := m.confirmWidth() - t.S.Dialog.GetHorizontalFrameSize()
	body = []string{t.S.Soft.Render(s.Env + " returns to the last healthy version."), ""}
	now := t.S.Base.Render(s.Version) + t.S.Subtle.Render("  deployed "+s.Deployed)
	if s.State == "error" {
		now = t.S.Err.Render(s.Version) + t.S.Subtle.Render("  failing 12 min")
	}
	body = append(body, ui.KV(t, [][2]string{
		{"now", now},
		{"after", t.S.OK.Render(s.Prev) + t.S.Subtle.Render("  the previous release")},
		{"impact", t.S.Base.Render("3 pods restart, one at a time (~90s)")},
	}, 7)...)
	btns = ui.Buttons(t, inner, lipgloss.Right,
		ui.Button(t, "Roll back", m.btn == 0, true, 0),
		ui.Button(t, "Cancel", m.btn == 1, false, 0))
	return "Roll back " + s.Name + "?", body, btns
}

// confirmRows is how many rows of the confirm body the dialog shows at the model's size.
func (m *app) confirmRows() int {
	title, _, btns := m.confirmBody()
	return ui.DialogBodyRows(m.t, dialogRoom(m.footerY()), title, btns, "")
}

func (m *app) confirm(c *ui.Canvas, fy int) {
	title, body, btns := m.confirmBody()
	m.confOff = min(m.confOff, ui.ClipMax(len(body), m.confirmRows()))
	d := ui.DialogScrolled(m.t, m.confirmWidth(), dialogRoom(fy), title, body, m.confOff, btns, "")
	m.ovRect = c.Overlay(d, dialogTop(c, lipgloss.Height(d), fy))
}

// helpBody is the full help's rows: every binding of the state underneath (the
// overlay help was opened from, or the screen), then ctrl+c.
func (m *app) helpBody() []string {
	t := m.t
	ov := m.ov
	m.ov = m.helpFrom
	acts := m.actions()
	m.ov = ov
	var body []string
	for _, a := range acts {
		h := a.b.Help()
		body = append(body, t.S.Key.Render(ui.Fit(h.Key, 12))+t.S.Soft.Render(h.Desc))
	}
	return append(body, t.S.Key.Render(ui.Fit("ctrl+c", 12))+t.S.Soft.Render("quit from anywhere"))
}

// helpTitle names the screen the help is for.
func (m *app) helpTitle() string {
	name := tabNames[min(int(m.screen), len(tabNames)-1)]
	switch m.screen {
	case scrDeploy:
		name = "Deploy"
	case scrLadder:
		name = "Legend"
	case scrSetup:
		name = "Setup"
	}
	switch m.helpFrom {
	case ovPalette:
		name = "Commands"
	case ovConfirm:
		name = "Roll back"
	}
	return "Keys " + m.t.G.Sep + " " + name
}

// footerY is the row the footer starts on at the model's size (draw's fy), computed
// here so the action table can tell whether a dialog's body scrolls before anything is
// drawn.
func (m *app) footerY() int {
	return m.h - len(ui.Footer(m.t, m.w, "", nil, ""))
}

// helpRows is how many rows of the help body the dialog shows at the model's size.
func (m *app) helpRows() int {
	return ui.DialogBodyRows(m.t, dialogRoom(m.footerY()), m.helpTitle(), "", "")
}

// keysOverlay is the full help: every binding of the screen underneath, scrolled
// (helpOff) when the screen's keys outnumber the rows the dialog has.
func (m *app) keysOverlay(c *ui.Canvas, fy int) {
	t := m.t
	w := min(56, c.Bounds().Dx()-6)
	body := m.helpBody()
	m.helpOff = min(m.helpOff, ui.ClipMax(len(body), m.helpRows()))
	d := ui.DialogScrolled(t, w, dialogRoom(fy), m.helpTitle(), body, m.helpOff, "", "")
	m.ovRect = c.Overlay(d, dialogTop(c, lipgloss.Height(d), fy))
}

// ---------------------------------------------------------------- splash

func (m *app) splash(c *ui.Canvas) {
	t := m.t
	b := c.Bounds()
	f := ui.DefaultField()
	f.ClearR, f.Dim, f.Drift = 0.42, 0.25, m.frame/3
	f.Draw(c, b)
	// a brand whose signature is its header (a flag strip, a stamp) shows it here too
	top := 0
	if t.X.Header == "bar" || t.X.Header == "stamp" {
		head := ui.Header(t, b.Dx(), b.Dy(), t.AppName(), t.S.Muted.Render("v0.9.0"))
		c.Draw(strings.Join(head, "\n"), uv.Rect(0, 0, b.Dx(), len(head)))
		top = len(head)
	}
	var block []string
	if rows := t.Wordmark.Rows; len(rows) > 0 && lipgloss.Width(rows[0]) < b.Dx()-4 {
		for _, r := range rows {
			block = append(block, bannerTone(t, r))
		}
	} else {
		block = append(block, ui.Wordmark(t, t.AppName()))
	}
	block = append(block, "", ui.NewAnim(t, 8, "connecting to prod", "splash").Frame(m.frame, ""))
	bw := 0
	for _, r := range block {
		bw = max(bw, lipgloss.Width(r))
	}
	for i := range block {
		block[i] = lipgloss.PlaceHorizontal(bw, lipgloss.Center, block[i])
	}
	y := max(top+1, (b.Dy()-len(block))/2-1)
	r := uv.Rect((b.Dx()-bw)/2, y, bw, len(block))
	c.Draw(strings.Join(block, "\n"), r)
	if top == 0 {
		c.Draw(t.S.Subtle.Render(t.AppName()+" v0.9.0"), uv.Rect(t.X.Gutter, b.Dy()-1, 30, 1))
	}
	q := t.S.Key.Render("q") + t.S.KeyDesc.Render(" quit")
	c.Draw(q, uv.Rect(b.Dx()-t.X.Gutter-lipgloss.Width(q), b.Dy()-1, lipgloss.Width(q), 1))
}

func bannerTone(t *theme.Theme, r string) string {
	var sb strings.Builder
	for _, ch := range r {
		switch string(ch) {
		case theme.ShadeDark:
			sb.WriteString(t.S.Primary.Render(theme.ShadeDark))
		case theme.ShadeMid:
			sb.WriteString(t.S.Muted.Render(theme.ShadeMid))
		case " ":
			sb.WriteRune(' ')
		default:
			sb.WriteString(t.S.Accent.Render(string(ch)))
		}
	}
	return sb.String()
}
