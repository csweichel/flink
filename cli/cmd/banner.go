package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiPurple = "\x1b[38;5;99m"
	ansiBlue   = "\x1b[38;5;39m"
	ansiCyan   = "\x1b[38;5;51m"
	ansiWhite  = "\x1b[38;5;255m"
	ansiInk    = "\x1b[38;5;15m"
)

func installBannerHelp(root *cobra.Command) {
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		w := cmd.OutOrStdout()
		PrintFlinkBanner(w)
		if description := strings.TrimSpace(firstNonEmpty(cmd.Long, cmd.Short)); description != "" {
			fmt.Fprintln(w, description)
			fmt.Fprintln(w)
		}
		fmt.Fprint(w, cmd.UsageString())
	})
}

func PrintFlinkBanner(w io.Writer) {
	fmt.Fprint(w, FlinkBanner(shouldUseColor(w)))
}

func FlinkBanner(color bool) string {
	c := func(code, s string) string {
		if !color {
			return s
		}
		return code + s + ansiReset
	}

	lines := []string{
		c(ansiPurple, "              ▟██████▙"),
		c(ansiPurple, "             ▟██") + c(ansiInk, "▀▀▀▀▀▀▀▀▘"),
		c(ansiPurple, "            ▟██") + c(ansiInk, "   ▟████▘"),
		c(ansiPurple, "       ●   ▟██") + c(ansiInk, "   ▟██"),
		c(ansiPurple, "     ▪ ▪  ▟██") + c(ansiInk, "   ▟██") + "        " + c(ansiBold+ansiWhite, "flink"),
		c(ansiBlue, "   ▪ ▪ ▪ ▟██") + c(ansiInk, "▄▄▄▟██") + "         " + c(ansiDim+ansiWhite, "live HTML/JS prototypes"),
		c(ansiBlue, " ▪ ▪ ▪ ▪ ▜█████▛") + "          " + c(ansiDim+ansiWhite, "publish • host • realtime"),
		c(ansiCyan, "   ▪ ▪ ▪   ▀▀▀"),
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
