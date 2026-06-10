package banner

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

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
	type rgb struct {
		r int
		g int
		b int
	}
	type segment struct {
		color rgb
		text  string
	}

	style := func(c rgb) func(string) string {
		if !colorEnabled {
			return func(s string) string { return s }
		}
		out := color.RGB(c.r, c.g, c.b)
		out.EnableColor()
		sprint := out.SprintFunc()
		return func(s string) string { return sprint(s) }
	}

	purple := rgb{112, 18, 232}
	violet := rgb{135, 39, 255}
	navy := rgb{7, 29, 50}
	blue := rgb{37, 110, 255}
	cyan := rgb{0, 211, 216}
	white := rgb{250, 250, 255}
	text := rgb{20, 24, 35}
	muted := rgb{70, 78, 95}

	palette := map[rgb]func(string) string{
		purple: style(purple),
		violet: style(violet),
		navy:   style(navy),
		blue:   style(blue),
		cyan:   style(cyan),
		white:  style(white),
		text:   style(text),
		muted:  style(muted),
	}

	line := func(parts ...segment) string {
		var b strings.Builder
		for _, part := range parts {
			if part.text == "" {
				continue
			}
			if paint, ok := palette[part.color]; ok {
				b.WriteString(paint(part.text))
			} else {
				b.WriteString(part.text)
			}
		}
		return b.String()
	}

	lines := []string{
		line(segment{text: "                    "}, segment{navy, "██████████████████████"}),
		line(segment{text: "                 "}, segment{navy, "████████████████████████"}),
		line(segment{text: "              "}, segment{purple, "██████"}, segment{navy, "        ████████"}),
		line(segment{text: "           "}, segment{violet, "██████"}, segment{navy, "     █████████"}),
		line(segment{text: "        "}, segment{violet, "██████"}, segment{navy, "  ████████████"}, segment{text: "      "}, segment{text, "flink"}),
		line(segment{text: "        "}, segment{violet, "██████"}, segment{navy, "██████████"}, segment{text: "        "}, segment{muted, "live HTML/JS prototypes"}),
		line(segment{text: "        "}, segment{violet, "██████"}, segment{navy, "     ██████"}, segment{text: "        "}, segment{muted, "publish • host • realtime"}),
		line(segment{text: "        "}, segment{violet, "███"}, segment{white, "●"}, segment{violet, "██"}, segment{navy, "     ██████"}),
		line(segment{text: "        "}, segment{purple, "██████"}, segment{navy, "  ██████"}),
		line(segment{text: "        "}, segment{purple, "████████████"}),
		line(segment{text: "          "}, segment{purple, "████████"}),
		line(segment{text: "     "}, segment{violet, "██"}, segment{text: "   "}, segment{violet, "██"}, segment{text: "   "}, segment{blue, "██"}),
		line(segment{text: "       "}, segment{violet, "████"}, segment{blue, "████████"}, segment{cyan, "██████"}),
		line(segment{text: "         "}, segment{blue, "████████████"}, segment{cyan, "████████"}),
		line(segment{text: "           "}, segment{blue, "████████"}, segment{cyan, "████████████"}),
	}

	return strings.Join(lines, "\n") + "\n"
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
