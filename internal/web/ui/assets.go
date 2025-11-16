package ui

import (
	"embed"
	"fmt"
	"html/template"
)

//go:embed css/*.css
var StaticFS embed.FS

//go:embed *.tmpl
var embeddedTemplatesFS embed.FS

func LoadTemplates() (*template.Template, error) {
	tmpl := template.New("")

	tmpl = tmpl.Funcs(
		template.FuncMap{
			"add":            Add,
			"formatDuration": FormatDuration,
			"formatTime":     FormatTime,
			"icon":           IncludeIcon,
		},
	)

	parsedTemplates, err := tmpl.ParseFS(embeddedTemplatesFS, "*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("failed to parse embedded templates: %w", err)
	}

	return parsedTemplates, nil
}

//go:embed icons/*.svg
var embeddedIconsFS embed.FS
