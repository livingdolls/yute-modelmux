package prompt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTemplate(t *testing.T) {
	dir := t.TempDir()
	content := `name: code-review
description: Review code for bugs
system: "You are a senior reviewer"
template: "Review: {{.input}}"
`
	os.WriteFile(filepath.Join(dir, "code-review.yaml"), []byte(content), 0o644)

	l := NewLoader(dir)
	tmpl, err := l.FindByName("code-review")
	if err != nil {
		t.Fatalf("FindByName failed: %v", err)
	}
	if tmpl.Name != "code-review" {
		t.Fatalf("expected code-review, got %s", tmpl.Name)
	}
	if tmpl.Description != "Review code for bugs" {
		t.Fatalf("unexpected description: %s", tmpl.Description)
	}
}

func TestRenderTemplate(t *testing.T) {
	dir := t.TempDir()
	content := `name: greet
system: "You are {{.input}} expert"
template: "Hello {{.input}}"
`
	os.WriteFile(filepath.Join(dir, "greet.yaml"), []byte(content), 0o644)

	l := NewLoader(dir)
	tmpl, err := l.FindByName("greet")
	if err != nil {
		t.Fatalf("FindByName failed: %v", err)
	}

	rendered, err := tmpl.Render("Go")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if rendered.User != "Hello Go" {
		t.Fatalf("expected 'Hello Go', got %q", rendered.User)
	}
	if rendered.System != "You are Go expert" {
		t.Fatalf("expected 'You are Go expert', got %q", rendered.System)
	}
}

func TestFindByNameNotFound(t *testing.T) {
	dir := t.TempDir()
	l := NewLoader(dir)
	_, err := l.FindByName("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
}

func TestLoadAll(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("name: a\ntemplate: hi"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.yaml"), []byte("name: b\ntemplate: hey"), 0o644)
	os.WriteFile(filepath.Join(dir, "not-a-template.txt"), []byte("hello"), 0o644)

	l := NewLoader(dir)
	templates, err := l.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}
	if len(templates) != 2 {
		t.Fatalf("expected 2 templates, got %d", len(templates))
	}
}
