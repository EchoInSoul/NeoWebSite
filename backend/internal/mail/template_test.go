package mail

import (
	"strings"
	"testing"
)

func TestRenderExample(t *testing.T) {
	body, err := Render("example", map[string]string{"Time": "2026-10-11 12:00:00", "Site": "NeoBlog"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if body.Text == "" || body.HTML == "" {
		t.Fatal("expected both text and html bodies")
	}
	for name, content := range map[string]string{"text": body.Text, "html": body.HTML} {
		if !strings.Contains(content, "2026-10-11 12:00:00") || !strings.Contains(content, "NeoBlog") {
			t.Fatalf("%s body missing rendered values: %s", name, content)
		}
	}
}

func TestRenderMissingTemplate(t *testing.T) {
	if _, err := Render("nope", nil); err == nil {
		t.Fatal("expected an error for a missing template")
	}
}

func TestRenderEscapesHTML(t *testing.T) {
	body, err := Render("example", map[string]string{"Time": "<script>alert(1)</script>", "Site": "NeoBlog"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(body.HTML, "<script>alert(1)</script>") {
		t.Fatalf("html body must escape script tags: %s", body.HTML)
	}
	if !strings.Contains(body.HTML, "&lt;script&gt;") {
		t.Fatalf("html body must contain the escaped script tag: %s", body.HTML)
	}
	if !strings.Contains(body.Text, "<script>alert(1)</script>") {
		t.Fatalf("text body must keep raw content: %s", body.Text)
	}
}
