package ui

import (
	"fmt"
	"html/template"
	"strings"
	"time"
)

var templateFuncMap = template.FuncMap{
	"add":            add,
	"formatDuration": formatDuration,
	"formatTime":     formatTime,
	"icon":           includeIcon,
}

func includeIcon(name string) (template.HTML, error) {
	iconPath := fmt.Sprintf("icons/%s.svg", name)
	data, err := embeddedIconsFS.ReadFile(iconPath)
	if err != nil {
		return "", err
	}

	// sec: template.HTML is safe here as the content is trusted
	return template.HTML(data), nil // #nosec G203
}

func add(a, b int) int {
	return a + b
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		return "N/A"
	}

	s := ""
	days := int(d.Hours()) / 24
	if days > 0 {
		s += fmt.Sprintf("%dd ", days)
		d -= time.Duration(days) * 24 * time.Hour
	}

	hours := int(d.Hours())
	if hours > 0 {
		s += fmt.Sprintf("%dh ", hours)
		d -= time.Duration(hours) * time.Hour
	}

	minutes := int(d.Minutes())
	if minutes > 0 {
		s += fmt.Sprintf("%dm ", minutes)
		d -= time.Duration(minutes) * time.Minute
	}

	seconds := int(d.Seconds())
	s += fmt.Sprintf("%ds", seconds)

	return strings.TrimSpace(s)
}

func formatTime(time *time.Time, format string) string {
	if time == nil {
		return "N/A"
	}

	return time.Format(format)
}
