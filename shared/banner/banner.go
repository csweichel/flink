package banner

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const (
	iconColumns = 30
	iconRows    = 16
)

//go:embed icon.png
var assets embed.FS

func InstallHelp(root *cobra.Command) {
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		w := cmd.OutOrStdout()
		Print(w)
		if description := strings.TrimSpace(firstNonEmpty(cmd.Long, cmd.Short)); description != "" {
			fmt.Fprintln(w, description)
			fmt.Fprintln(w)
		}
		fmt.Fprint(w, cmd.UsageString())
	})
}

func Print(w io.Writer) {
	fmt.Fprint(w, Render(shouldUseColor(w)))
}

func Render(colorEnabled bool) string {
	lines, err := renderIcon(colorEnabled)
	if err != nil {
		lines = fallbackIcon()
	}

	if colorEnabled {
		title := color.RGB(250, 250, 255)
		title.EnableColor()
		muted := color.RGB(160, 170, 188)
		muted.EnableColor()
		addLabel(lines, title.Sprint("flink"), 4)
		addLabel(lines, muted.Sprint("live HTML/JS prototypes"), 5)
		addLabel(lines, muted.Sprint("publish • host • realtime"), 6)
	} else {
		addLabel(lines, "flink", 4)
		addLabel(lines, "live HTML/JS prototypes", 5)
		addLabel(lines, "publish • host • realtime", 6)
	}

	return strings.Join(lines, "\n") + "\n"
}

func renderIcon(colorEnabled bool) ([]string, error) {
	b, err := assets.ReadFile("icon.png")
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}

	crop, ok := visibleBounds(img)
	if !ok {
		return nil, fmt.Errorf("icon has no visible pixels")
	}

	targetWidth := iconColumns
	targetHeight := iconRows * 2

	pixels := make([][]pixel, targetHeight)
	for y := range pixels {
		pixels[y] = make([]pixel, targetWidth)
		for x := range pixels[y] {
			pixels[y][x] = sample(img, crop, targetWidth, targetHeight, x, y)
		}
	}

	lines := make([]string, 0, targetHeight/2)
	for y := 0; y < targetHeight; y += 2 {
		if colorEnabled {
			lines = append(lines, colorLine(pixels[y], pixels[y+1]))
		} else {
			lines = append(lines, plainLine(pixels[y], pixels[y+1]))
		}
	}
	return trimEmptyLines(lines), nil
}

type pixel struct {
	r uint8
	g uint8
	b uint8
	a uint8
}

func visibleBounds(img image.Image) (image.Rectangle, bool) {
	bounds := img.Bounds()
	minX, minY := bounds.Max.X, bounds.Max.Y
	maxX, maxY := bounds.Min.X, bounds.Min.Y

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if !visible(pixelAt(img, x, y)) {
				continue
			}
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x+1 > maxX {
				maxX = x + 1
			}
			if y+1 > maxY {
				maxY = y + 1
			}
		}
	}
	if maxX <= minX || maxY <= minY {
		return image.Rectangle{}, false
	}
	padX := max(1, (maxX-minX)/28)
	padY := max(1, (maxY-minY)/28)
	return image.Rect(
		max(bounds.Min.X, minX-padX),
		max(bounds.Min.Y, minY-padY),
		min(bounds.Max.X, maxX+padX),
		min(bounds.Max.Y, maxY+padY),
	), true
}

func sample(img image.Image, crop image.Rectangle, width, height, x, y int) pixel {
	x0 := crop.Min.X + x*crop.Dx()/width
	x1 := crop.Min.X + (x+1)*crop.Dx()/width
	y0 := crop.Min.Y + y*crop.Dy()/height
	y1 := crop.Min.Y + (y+1)*crop.Dy()/height
	if x1 <= x0 {
		x1 = x0 + 1
	}
	if y1 <= y0 {
		y1 = y0 + 1
	}

	var rs, gs, bs, as uint64
	var count uint64
	for sy := y0; sy < y1; sy++ {
		for sx := x0; sx < x1; sx++ {
			p := pixelAt(img, sx, sy)
			if !visible(p) {
				continue
			}
			rs += uint64(p.r)
			gs += uint64(p.g)
			bs += uint64(p.b)
			as += uint64(p.a)
			count++
		}
	}
	if count == 0 {
		return pixel{}
	}
	return pixel{
		r: uint8(rs / count),
		g: uint8(gs / count),
		b: uint8(bs / count),
		a: uint8(as / count),
	}
}

func pixelAt(img image.Image, x, y int) pixel {
	r, g, b, a := img.At(x, y).RGBA()
	return pixel{
		r: uint8(r >> 8),
		g: uint8(g >> 8),
		b: uint8(b >> 8),
		a: uint8(a >> 8),
	}
}

func visible(p pixel) bool {
	return p.a > 20 && max(int(p.r), int(p.g), int(p.b)) > 8
}

func colorLine(top, bottom []pixel) string {
	var b strings.Builder
	for i := range top {
		t := top[i]
		bt := bottom[i]
		switch {
		case visible(t) && visible(bt):
			c := color.RGB(int(t.r), int(t.g), int(t.b)).AddBgRGB(int(bt.r), int(bt.g), int(bt.b))
			c.EnableColor()
			b.WriteString(c.Sprint("▀"))
		case visible(t):
			c := color.RGB(int(t.r), int(t.g), int(t.b))
			c.EnableColor()
			b.WriteString(c.Sprint("▀"))
		case visible(bt):
			c := color.RGB(int(bt.r), int(bt.g), int(bt.b))
			c.EnableColor()
			b.WriteString(c.Sprint("▄"))
		default:
			b.WriteByte(' ')
		}
	}
	return strings.TrimRight(b.String(), " ")
}

func plainLine(top, bottom []pixel) string {
	var b strings.Builder
	for i := range top {
		l := max(luma(top[i]), luma(bottom[i]))
		switch {
		case l < 12:
			b.WriteByte(' ')
		case l < 55:
			b.WriteRune('░')
		case l < 105:
			b.WriteRune('▒')
		case l < 170:
			b.WriteRune('▓')
		default:
			b.WriteRune('█')
		}
	}
	return strings.TrimRight(b.String(), " ")
}

func luma(p pixel) int {
	if !visible(p) {
		return 0
	}
	return (299*int(p.r) + 587*int(p.g) + 114*int(p.b)) / 1000
}

func trimEmptyLines(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[start:end]
}

func addLabel(lines []string, text string, line int) {
	if line < 0 || line >= len(lines) {
		return
	}
	width := utf8.RuneCountInString(stripANSI(lines[line]))
	lines[line] = lines[line] + strings.Repeat(" ", max(2, iconColumns-width+4)) + text
}

func stripANSI(s string) string {
	var out strings.Builder
	inEscape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inEscape {
			if c >= '@' && c <= '~' {
				inEscape = false
			}
			continue
		}
		if c == 0x1b {
			inEscape = true
			continue
		}
		out.WriteByte(c)
	}
	return out.String()
}

func fallbackIcon() []string {
	return []string{
		"        ▄████████▄",
		"      ▄██▀      ▀██▄",
		"    ▄██▀  ▄████▄",
		"  ▄██▀   ▄██▀",
		"  ██●   ▄██▄▄",
		"  ██▄▄▄██▀▀",
		"    ▀▀▀",
		"  ▄▄ ▄▄ ▄▄",
		"    ▀████████▄",
	}
}

func shouldUseColor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
