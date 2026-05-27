package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestBuildMermaid(t *testing.T) {
	modules := []Module{
		{
			Name: "mod1",
			Versions: []Version{
				{
					Name: "1.0.0",
					Dependencies: []Dependency{
						{Name: "mod2", Version: "2.0.0"},
					},
				},
			},
		},
		{
			Name: "mod2",
			Versions: []Version{
				{Name: "2.0.0"},
			},
		},
	}

	mermaid := buildMermaid(modules)
	if !strings.Contains(mermaid, "mod1(\"mod1<br/>1.0.0\")") {
		t.Errorf("Expected mermaid to contain node label for mod1, got: %s", mermaid)
	}
	if !strings.Contains(mermaid, "mod1 -- \"jump\" --> mod2") {
		t.Errorf("Expected mermaid to contain edge mod1 -- \"jump\" --> mod2, got: %s", mermaid)
	}
}

func TestGenerateHTML_Escaping(t *testing.T) {
	modules := []Module{}
	mermaid := "graph TD\n    A(\"Node A\")"
	
	var buf bytes.Buffer
	// We need a dummy WriteCloser
	wc := &dummyWriteCloser{Buffer: &buf}
	
	if err := generateHTML(modules, mermaid, wc); err != nil {
		t.Fatalf("generateHTML failed: %v", err)
	}
	
	output := buf.String()
	// Check if the mermaid block is escaped. 
	// If it is escaped, " will be &#34; or similar.
	if strings.Contains(output, "A(&#34;Node A&#34;)") || strings.Contains(output, "A(&#34;") {
		t.Errorf("Mermaid output seems to be HTML-escaped: %s", output)
	}
	
	if !strings.Contains(output, "A(\"Node A\")") {
		t.Errorf("Expected unescaped mermaid output, got: %s", output)
	}
}

type dummyWriteCloser struct {
	*bytes.Buffer
}

func (d *dummyWriteCloser) Close() error { return nil }

func TestBuildMermaid_Features(t *testing.T) {
	modules := []Module{
		{
			Name: "mod1",
			Versions: []Version{
				{
					Name: "1.0.0",
					Dependencies: []Dependency{
						{Name: "mod2", Version: "2.0.0"},
					},
				},
			},
		},
		{
			Name: "mod2",
			Versions: []Version{
				{Name: "2.0.0"},
			},
		},
	}

	mermaid := buildMermaid(modules)
	
	// Check for jump label on edge - correctly formatted
	if !strings.Contains(mermaid, "mod1 -- \"jump\" --> mod2") {
		t.Errorf("Expected jump label on edge mod1 -> mod2, got: %s", mermaid)
	}
	
	// Check for click command
	if !strings.Contains(mermaid, "click mod1 \"#card-mod1\"") {
		t.Errorf("Expected click command for mod1, got: %s", mermaid)
	}
	
	// Check for leaf class
	if !strings.Contains(mermaid, "class mod2 leaf") {
		t.Errorf("Expected mod2 to be a leaf, got: %s", mermaid)
	}
}
