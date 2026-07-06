package prompt

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

type Template struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	System      string `yaml:"system"`
	Template    string `yaml:"template"`
	Model       string `yaml:"model"`
	filename    string
}

type Loader struct {
	dir string
}

func NewLoader(dir string) *Loader {
	return &Loader{dir: dir}
}

func (l *Loader) LoadAll() ([]Template, error) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return nil, fmt.Errorf("prompt: read dir %s: %w", l.dir, err)
	}

	var templates []Template
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		tmpl, err := l.Load(filepath.Join(l.dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("prompt: load %s: %w", entry.Name(), err)
		}
		templates = append(templates, *tmpl)
	}
	return templates, nil
}

func (l *Loader) Load(path string) (*Template, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tmpl Template
	if err := yaml.Unmarshal(data, &tmpl); err != nil {
		return nil, fmt.Errorf("prompt: parse %s: %w", path, err)
	}
	tmpl.filename = filepath.Base(path)
	return &tmpl, nil
}

func (l *Loader) FindByName(name string) (*Template, error) {
	templates, err := l.LoadAll()
	if err != nil {
		return nil, err
	}
	for _, tmpl := range templates {
		if tmpl.Name == name {
			return &tmpl, nil
		}
	}
	return nil, fmt.Errorf("prompt: template %q not found", name)
}

func (t *Template) Render(input string) (*RenderedPrompt, error) {
	var userBuf bytes.Buffer
	tmpl, err := template.New(t.Name).Parse(t.Template)
	if err != nil {
		return nil, fmt.Errorf("prompt: parse template %s: %w", t.Name, err)
	}
	if err := tmpl.Execute(&userBuf, map[string]string{"input": input}); err != nil {
		return nil, fmt.Errorf("prompt: render template %s: %w", t.Name, err)
	}

	var systemBuf bytes.Buffer
	if t.System != "" {
		sysTmpl, err := template.New(t.Name + "-system").Parse(t.System)
		if err != nil {
			return nil, fmt.Errorf("prompt: parse system template: %w", err)
		}
		if err := sysTmpl.Execute(&systemBuf, map[string]string{"input": input}); err != nil {
			return nil, fmt.Errorf("prompt: render system template: %w", err)
		}
	}

	return &RenderedPrompt{
		System: systemBuf.String(),
		User:   userBuf.String(),
		Model:  t.Model,
	}, nil
}

type RenderedPrompt struct {
	System string
	User   string
	Model  string
}
