package cellshot

// The ANSI frame parser: text and SGR in, a grid of styled cells out. It accepts what
// cellshot.py accepts and warns about the same things. Widths follow the kit
// (charmbracelet/x/ansi grapheme widths), so a wide character takes two cells exactly
// when lipgloss thinks it does.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// RGB is a colour.
type RGB struct{ R, G, B uint8 }

// Color is an RGB colour, or the terminal default when !Set.
type Color struct {
	RGB
	Set bool
}

func rgb(r, g, b int) Color {
	c := func(v int) uint8 { return uint8(max(0, min(255, v))) }
	return Color{RGB{c(r), c(g), c(b)}, true}
}

// Style is the SGR state a cell was printed with.
type Style struct {
	FG, BG  Color
	Bold    bool
	Faint   bool
	Italic  bool
	Under   int // 0 none, 1 single, 2 double, 3 curly (others draw as single)
	Strike  bool
	Reverse bool
}

// Cell is one terminal cell. Text is a whole grapheme cluster; a wide cluster is
// followed by a Cont cell for its right half.
type Cell struct {
	Text string
	Style
	Wide bool
	Cont bool
}

var basic16 = [16]RGB{{0, 0, 0}, {205, 49, 49}, {13, 188, 121}, {229, 229, 16}, {36, 114, 200}, {188, 63, 188},
	{17, 168, 205}, {229, 229, 229}, {102, 102, 102}, {241, 76, 76}, {35, 209, 139}, {245, 245, 67},
	{59, 142, 234}, {214, 112, 214}, {41, 184, 219}, {255, 255, 255}}

// Basic16 is the colour cellshot draws ANSI colour n (0-15) in: VS Code's terminal
// palette, the one every sheet shows. palette check judges a 16-colour pin's contrast in
// it (palette.Inks16), so the number it prints is the one the sheet shows.
func Basic16(n int) RGB { return basic16[n&15] }

// xterm maps a 256-colour index to RGB the way xterm does.
func xterm(n int) Color {
	switch {
	case n < 0:
		n = 0
	case n < 16:
		return Color{basic16[n], true}
	case n < 232:
		n -= 16
		v := func(x int) int {
			if x == 0 {
				return 0
			}
			return 55 + x*40
		}
		return rgb(v(n/36), v(n/6%6), v(n%6))
	}
	g := 8 + (n-232)*10
	return rgb(g, g, g)
}

// isEmoji is cellshot.py's test: the pictographic planes, or a wide dingbat/symbol.
func isEmoji(r rune) bool {
	return (r >= 0x1F300 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF && ansi.StringWidth(string(r)) == 2)
}

// Parse turns a flat ANSI frame into rows of cells, plus sorted warnings about things
// that make it a poor render (emoji, non-SGR escapes).
func Parse(text string) (rows [][]Cell, warns []string) {
	text = strings.ToValidUTF8(text, "�")
	ws := map[string]bool{}
	var row []Cell
	var st Style
	n := len(text)
	for i := 0; i < n; {
		c := text[i]
		switch {
		case c == 0x1b:
			end, params, final := escape(text, i)
			sgr := final == 'm' && strings.Trim(params, "0123456789;:") == ""
			if sgr {
				applySGR(params, &st)
			} else if final != 0 && final != 'h' && final != 'l' {
				ws[fmt.Sprintf("non-SGR CSI '%c' ignored (cursor movement? frame may not be a flat render)", final)] = true
			}
			i = end
			continue
		case c == '\n':
			rows = append(rows, row)
			row = nil
			i++
			continue
		case c == '\r':
			i++
			continue
		case c == '\t':
			for k := 8 - len(row)%8; k > 0; k-- {
				row = append(row, Cell{Text: " ", Style: st})
			}
			i++
			continue
		case c < 0x20 || c == 0x7f:
			ws[fmt.Sprintf("control character U+%04X ignored (frame may not be a flat render)", c)] = true
			i++
			continue
		}
		j := i
		for j < n {
			b := text[j]
			if b < 0x20 || b == 0x7f {
				break
			}
			j++
		}
		run := text[i:j]
		for run != "" {
			cluster, w := ansi.FirstGraphemeCluster(run, ansi.GraphemeWidth)
			if cluster == "" {
				_, sz := utf8.DecodeRuneInString(run)
				cluster = run[:sz]
			}
			run = run[len(cluster):]
			if w == 0 {
				for k := len(row) - 1; k >= 0; k-- { // combining marks join the cell before
					if !row[k].Cont {
						row[k].Text += cluster
						break
					}
				}
				continue
			}
			base, _ := utf8.DecodeRuneInString(cluster)
			if isEmoji(base) {
				ws["emoji present (rendering varies by terminal and font; usually a design smell)"] = true
			}
			if w >= 2 {
				row = append(row, Cell{Text: cluster, Style: st, Wide: true}, Cell{Style: st, Cont: true})
			} else {
				row = append(row, Cell{Text: cluster, Style: st})
			}
		}
		i = j
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	for w := range ws {
		warns = append(warns, w)
	}
	sort.Strings(warns)
	return rows, warns
}

// escape matches one escape sequence at text[i] (an ESC) the way cellshot.py's regex
// does, except that a CSI may also carry intermediate bytes (CSI SP q). It returns
// where the sequence ends and, for a CSI, its parameter and intermediate bytes and its
// final byte (final is 0 for anything else). An ESC that starts nothing is skipped
// alone. A CSI ending in m with a private marker (CSI > 4 ; 2 m) is not SGR and is
// reported like any other non-SGR CSI.
func escape(text string, i int) (end int, params string, final byte) {
	n := len(text)
	if i+1 >= n {
		return i + 1, "", 0
	}
	switch text[i+1] {
	case '[':
		j := i + 2
		for j < n && strings.IndexByte("0123456789;:?<>=", text[j]) >= 0 {
			j++
		}
		for j < n && text[j] >= ' ' && text[j] <= '/' { // intermediates: CSI SP q, CSI ! p
			j++
		}
		if j < n && text[j] >= '@' && text[j] <= '~' {
			return j + 1, text[i+2 : j], text[j]
		}
		return i + 1, "", 0
	case ']':
		j := i + 2
		for j < n && text[j] != 0x07 && text[j] != 0x1b {
			j++
		}
		if j < n && text[j] == 0x07 {
			return j + 1, "", 0
		}
		if j+1 < n && text[j] == 0x1b && text[j+1] == '\\' {
			return j + 2, "", 0
		}
		return i + 2, "", 0 // unterminated OSC: only ESC ] is consumed
	case 'P', 'X', '^', '_':
		j := strings.IndexByte(text[i+2:], 0x1b)
		if j >= 0 && i+2+j+1 < n && text[i+2+j+1] == '\\' {
			return i + 2 + j + 2, "", 0
		}
		return i + 2, "", 0
	}
	if b := text[i+1]; (b >= '@' && b <= 'Z') || (b >= '\\' && b <= '_') {
		return i + 2, "", 0
	}
	return i + 1, "", 0
}

func atoi(s string) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return v
}

// applySGR applies one SGR parameter string, with cellshot.py's coverage: 16/256/
// truecolour (semicolon and colon forms), bold, faint, italic, underline styles,
// strike, reverse and the resets.
func applySGR(params string, st *Style) {
	codes := []string{"0"}
	if params != "" {
		codes = strings.Split(params, ";")
	}
	for j := 0; j < len(codes); j++ {
		part := codes[j]
		if strings.Contains(part, ":") { // 38:2::r:g:b, 4:3 (curly underline)
			sub := strings.Split(part, ":")
			switch head := atoi(sub[0]); head {
			case 38, 48, 58:
				var vals []int
				for _, s := range sub[1:] {
					if s != "" {
						vals = append(vals, atoi(s))
					}
				}
				var col Color
				if len(vals) >= 2 && vals[0] == 5 {
					col = xterm(vals[1])
				} else if len(vals) >= 4 && vals[0] == 2 {
					v := vals[len(vals)-3:]
					col = rgb(v[0], v[1], v[2])
				}
				if head == 38 {
					st.FG = col
				} else if head == 48 {
					st.BG = col
				}
			case 4:
				st.Under = 1
				if len(sub) > 1 && sub[1] != "" {
					st.Under = atoi(sub[1])
				}
			}
			continue
		}
		c := atoi(part)
		switch {
		case c == 0:
			*st = Style{}
		case c == 1:
			st.Bold = true
		case c == 2:
			st.Faint = true
		case c == 3:
			st.Italic = true
		case c == 4:
			st.Under = 1
		case c == 21:
			st.Under = 2
		case c == 7:
			st.Reverse = true
		case c == 9:
			st.Strike = true
		case c == 22:
			st.Bold, st.Faint = false, false
		case c == 23:
			st.Italic = false
		case c == 24:
			st.Under = 0
		case c == 27:
			st.Reverse = false
		case c == 29:
			st.Strike = false
		case c >= 30 && c <= 37:
			st.FG = Color{basic16[c-30], true}
		case c >= 90 && c <= 97:
			st.FG = Color{basic16[c-90+8], true}
		case c >= 40 && c <= 47:
			st.BG = Color{basic16[c-40], true}
		case c >= 100 && c <= 107:
			st.BG = Color{basic16[c-100+8], true}
		case c == 39:
			st.FG = Color{}
		case c == 49:
			st.BG = Color{}
		case (c == 38 || c == 48 || c == 58) && j+1 < len(codes):
			mode := atoi(codes[j+1])
			var col Color
			if mode == 5 && j+2 < len(codes) {
				col = xterm(atoi(codes[j+2]))
				j += 2
			} else if mode == 2 && j+4 < len(codes) {
				col = rgb(atoi(codes[j+2]), atoi(codes[j+3]), atoi(codes[j+4]))
				j += 4
			}
			if c == 38 {
				st.FG = col
			} else if c == 48 {
				st.BG = col
			}
		}
	}
}
