package mail

import (
	"bytes"
	"embed"
	"fmt"
	htmltemplate "html/template"
	texttemplate "text/template"
)

//go:embed templates/*.html templates/*.txt
var templateFS embed.FS

func Render(name string, data any) (Body, error) {
	htmlTemplate, err := htmltemplate.ParseFS(templateFS, "templates/"+name+".html")
	if err != nil {
		return Body{}, fmt.Errorf("mail: render %s html: %w", name, err)
	}

	textTemplate, err := texttemplate.ParseFS(templateFS, "templates/"+name+".txt")
	if err != nil {
		return Body{}, fmt.Errorf("mail: render %s text: %w", name, err)
	}

	var htmlBuffer bytes.Buffer
	if err := htmlTemplate.Execute(&htmlBuffer, data); err != nil {
		return Body{}, fmt.Errorf("mail: render %s html: %w", name, err)
	}

	var textBuffer bytes.Buffer
	if err := textTemplate.Execute(&textBuffer, data); err != nil {
		return Body{}, fmt.Errorf("mail: render %s text: %w", name, err)
	}

	return Body{Text: textBuffer.String(), HTML: htmlBuffer.String()}, nil
}
