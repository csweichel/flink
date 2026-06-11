package cmd

import (
	"fmt"
	"io"
	"strings"
)

type outputSection struct {
	Title string
	Rows  []outputRow
}

type outputRow struct {
	Label string
	Value string
}

func printSections(w io.Writer, title string, sections ...outputSection) {
	if strings.TrimSpace(title) != "" {
		fmt.Fprintln(w, title)
	}
	for _, section := range sections {
		if len(section.Rows) == 0 {
			continue
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, section.Title)
		for _, row := range section.Rows {
			if strings.TrimSpace(row.Value) == "" {
				row.Value = "-"
			}
			fmt.Fprintf(w, "  %-12s %s\n", row.Label, row.Value)
		}
	}
}

func row(label string, value any) outputRow {
	return outputRow{Label: label, Value: fmt.Sprint(value)}
}

func optionalRow(label, value string) []outputRow {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return []outputRow{row(label, value)}
}
