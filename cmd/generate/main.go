package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	bazelDepRe = regexp.MustCompile(`(?s)bazel_dep\s*\((.*?)\)`)
	nameRe     = regexp.MustCompile(`name\s*=\s*"([^"]+)"`)
	versionRe  = regexp.MustCompile(`version\s*=\s*"([^"]+)"`)
	devDepRe   = regexp.MustCompile(`dev_dependency\s*=\s*True`)
)

const (
	outputFile = "index.html"
)

type Module struct {
	Name     string
	Metadata Metadata
	Versions []Version
}

type Metadata struct {
	Homepage       string            `json:"homepage"`
	Repo           []string          `json:"repository"`
	Versions       []string          `json:"versions"`
	YankedVersions map[string]string `json:"yanked_versions"`
}

type Version struct {
	Name         string
	ModuleFile   string
	SourceFile   string
	Dependencies []Dependency
}

type Dependency struct {
	Name          string
	Version       string
	DevDependency bool
}

type TemplateData struct {
	Modules []Module
	Mermaid template.HTML
}

func main() {
	var (
		modulesDir string
		outputFile string
		mode       string
	)
	flag.StringVar(&modulesDir, "modules_dir", "", "The path to the modules directory.")
	flag.StringVar(&outputFile, "output", "", "The file name to output")
	flag.StringVar(&mode, "mode", "html", "The output mode: html or mermaid")
	flag.Parse()
	if modulesDir == "" {
		log.Printf("flag --modules_dir=... is required")
		os.Exit(1)
	}
	if outputFile == "" {
		log.Printf("flag --output=... is required")
		os.Exit(1)
	}

	if err := run(modulesDir, outputFile, mode); err != nil {
		log.Printf("error: %v", err)
		os.Exit(1)
	}
}

func run(modulesDir, outputFile, mode string) error {
	modules, err := findModules(modulesDir)
	if err != nil {
		log.Fatalf("failed to find modules: %v", err)
	}

	o, err := os.Create(outputFile)
	if err != nil {
		log.Printf("could not create: %v: %v", outputFile, err)
	}

	mermaid := buildMermaid(modules)

	if mode == "mermaid" {
		if _, err := o.Write([]byte(mermaid)); err != nil {
			log.Fatalf("failed to write mermaid: %v", err)
		}
	} else {
		if err := generateHTML(modules, mermaid, o); err != nil {
			log.Fatalf("failed to generate HTML: %v", err)
		}
	}

	return nil
}

func buildMermaid(modules []Module) string {
	var sb strings.Builder
	sb.WriteString(`---
config:
  layout: elk
  elk:
    mergeEdges: true
---
`)
	sb.WriteString("flowchart TB\n")

	registryLatest := make(map[string]string)
	for _, m := range modules {
		if len(m.Versions) > 0 {
			registryLatest[m.Name] = m.Versions[0].Name
		}
	}

	nodes := make(map[string]bool)
	edges := make(map[string]bool)
	allNodes := make(map[string]bool)

	// Collect all external modules
	externalModulesSet := make(map[string]string) // Name -> Version
	for _, m := range modules {
		if len(m.Versions) == 0 {
			continue
		}
		latest := m.Versions[0]
		for _, dep := range latest.Dependencies {
			if _, ok := registryLatest[dep.Name]; !ok {
				externalModulesSet[dep.Name] = dep.Version
			}
		}
	}

	var externalNames []string
	for name := range externalModulesSet {
		externalNames = append(externalNames, name)
	}
	sort.Strings(externalNames)

	escape := func(s string) string {
		return strings.ReplaceAll(s, "\"", "\\\"")
	}

	externalNodeID := "ExternalModules"
	if len(externalNames) > 0 {
		// One entry per line made this node far taller than the rest of the
		// graph. List the entries inline instead, and only wrap when a line
		// reaches the width budget, so the node comes out about as wide as
		// the graph and only a few lines tall.
		const lineWidthBudget = 570 // characters per label line; sized so the node spans roughly the graph width
		var lines []string
		var line string
		for _, name := range externalNames {
			entry := fmt.Sprintf("%s (%s)", name, externalModulesSet[name])
			if line == "" {
				line = entry
			} else if len(line)+len(" · ")+len(entry) <= lineWidthBudget {
				line += " · " + entry
			} else {
				lines = append(lines, line)
				line = entry
			}
		}
		if line != "" {
			lines = append(lines, line)
		}
		label := strings.Join(lines, "\n")
		// A markdown-string label (backticks): unlike classic labels, its
		// wrapping is governed by markdownAutoWrap, which the page disables,
		// so the generator's own line breaks are the only ones.
		sb.WriteString(fmt.Sprintf("    %s[\"`%s`\"]\n", externalNodeID, escape(label)))
		sb.WriteString(fmt.Sprintf("    class %s inverted\n", externalNodeID))
		allNodes[externalNodeID] = true
	}

	for _, m := range modules {
		if len(m.Versions) == 0 {
			continue
		}
		latest := m.Versions[0]
		mID := sanitizeID(m.Name)
		if !nodes[mID] {
			sb.WriteString(fmt.Sprintf("    %s[\"%s\n%s\"]\n", mID, escape(m.Name), escape(latest.Name)))
			nodes[mID] = true
			allNodes[mID] = true
		}

		hasInternalDeps := false
		for _, dep := range latest.Dependencies {
			var depID string
			if _, ok := registryLatest[dep.Name]; ok {
				depID = sanitizeID(dep.Name)
				hasInternalDeps = true
				if !nodes[depID] {
					version := registryLatest[dep.Name]
					sb.WriteString(fmt.Sprintf("    %s[\"%s\n%s\"]\n", depID, escape(dep.Name), escape(version)))
					nodes[depID] = true
					allNodes[depID] = true
				}
			} else {
				depID = externalNodeID
			}

			edgeID := fmt.Sprintf("%s->%s", mID, depID)
			if !edges[edgeID] {
				// Use "jump" label on edges for navigation
				sb.WriteString(fmt.Sprintf("    %s -- \"jump\" --> %s\n", mID, depID))
				edges[edgeID] = true
			}
		}

		if !hasInternalDeps {
			sb.WriteString(fmt.Sprintf("    class %s leaf\n", mID))
		}
	}

	for nID := range allNodes {
		if nID == externalNodeID {
			continue
		}
		sb.WriteString(fmt.Sprintf("    click %s \"#card-%s\"\n", nID, nID))
	}

	sb.WriteString("    classDef inverted fill:#333,color:#fff\n")
	sb.WriteString("    classDef leaf fill:#28a745,color:#fff\n")
	return sb.String()
}

var sanitizeRe = regexp.MustCompile("[^a-zA-Z0-9_]")

func sanitizeID(s string) string {
	return sanitizeRe.ReplaceAllString(s, "_")
}

func findModules(dir string) ([]Module, error) {
	var modules []Module

	moduleDirs, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read modules directory: %w", err)
	}

	for _, moduleDir := range moduleDirs {
		if !moduleDir.IsDir() {
			continue
		}

		modulePath := filepath.Join(dir, moduleDir.Name())
		metadataPath := filepath.Join(modulePath, "metadata.json")

		metadataFile, err := os.Open(metadataPath)
		if err != nil {
			log.Printf("skipping directory %s: metadata.json not found", moduleDir.Name())
			continue
		}
		defer metadataFile.Close()

		var metadata Metadata
		if err := json.NewDecoder(metadataFile).Decode(&metadata); err != nil {
			log.Printf("skipping directory %s: failed to parse metadata.json: %v", moduleDir.Name(), err)
			continue
		}

		versions, err := findVersions(modulePath)
		if err != nil {
			log.Printf("skipping directory %s: failed to find versions: %v", moduleDir.Name(), err)
			continue
		}

		modules = append(modules, Module{
			Name:     moduleDir.Name(),
			Metadata: metadata,
			Versions: versions,
		})
	}

	sort.Slice(modules, func(i, j int) bool {
		return modules[i].Name < modules[j].Name
	})

	return modules, nil
}

func findVersions(modulePath string) ([]Version, error) {
	var versions []Version

	versionDirs, err := ioutil.ReadDir(modulePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read module directory: %w", err)
	}

	for _, versionDir := range versionDirs {
		if !versionDir.IsDir() {
			continue
		}

		versionPath := filepath.Join(modulePath, versionDir.Name())
		moduleFilePath := filepath.Join(versionPath, "MODULE.bazel")
		sourceFilePath := filepath.Join(versionPath, "source.json")

		if _, err := os.Stat(moduleFilePath); os.IsNotExist(err) {
			continue
		}
		if _, err := os.Stat(sourceFilePath); os.IsNotExist(err) {
			continue
		}

		moduleFileContent, err := ioutil.ReadFile(moduleFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read MODULE.bazel: %w", err)
		}

		sourceFileContent, err := ioutil.ReadFile(sourceFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read source.json: %w", err)
		}

		var deps []Dependency
		matches := bazelDepRe.FindAllStringSubmatch(string(moduleFileContent), -1)
		for _, match := range matches {
			content := match[1]
			nameMatch := nameRe.FindStringSubmatch(content)
			versionMatch := versionRe.FindStringSubmatch(content)
			if len(nameMatch) > 1 && len(versionMatch) > 1 {
				isDev := devDepRe.MatchString(content)
				deps = append(deps, Dependency{
					Name:          nameMatch[1],
					Version:       versionMatch[1],
					DevDependency: isDev,
				})
			}
		}

		sort.Slice(deps, func(i, j int) bool {
			return deps[i].Name < deps[j].Name
		})

		versions = append(versions, Version{
			Name:         versionDir.Name(),
			ModuleFile:   string(moduleFileContent),
			SourceFile:   string(sourceFileContent),
			Dependencies: deps,
		})
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Name > versions[j].Name
	})

	return versions, nil
}

func generateHTML(modules []Module, mermaid string, w io.WriteCloser) error {
	defer w.Close()
	tmpl, err := template.New("index").Funcs(template.FuncMap{
		"isURL": func(s string) bool {
			return strings.HasPrefix(s, "http")
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"repoURL": func(s string) string {
			if strings.HasPrefix(s, "github:") {
				return "https://github.com/" + strings.TrimPrefix(s, "github:")
			}
			return s
		},
		"bazelDep": func(name, version string) string {
			return fmt.Sprintf(`bazel_dep(name = "%s", version = "%s")`, name, version)
		},
		"isYanked": func(version string, metadata Metadata) bool {
			_, ok := metadata.YankedVersions[version]
			return ok
		},
		"sanitizeID": sanitizeID,
	}).Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse HTML template: %w", err)
	}

	var buf strings.Builder
	data := TemplateData{
		Modules: modules,
		Mermaid: template.HTML(mermaid),
	}
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute HTML template: %w", err)
	}

	_, err = fmt.Fprintf(w, "%s", buf.String())
	if err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	return nil
}

const htmlTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Bazel Registry | HDL Factory</title>
    <!-- Styled by the site theme (hdlfactory.com.template, themes/hdlfactory);
         this page is served from /bazel-registry/ on the same host. -->
    <link rel="stylesheet" href="/css/theme.css">
    <link rel="icon" href="/logo/hf-mark.svg" type="image/svg+xml">
    <!-- Google tag (gtag.js) -->
    <script async src="https://www.googletagmanager.com/gtag/js?id=G-BKGTF9GD1K"></script>
    <script>
      window.dataLayer = window.dataLayer || [];
      function gtag(){dataLayer.push(arguments);}
      gtag('js', new Date());
      gtag('config', 'G-BKGTF9GD1K');
    </script>
    <script>
      // Same three-state theme resolution as the site theme, sharing the
      // "theme" localStorage key, so the choice follows the reader across
      // the whole site. Runs before first paint.
      (function () {
        function resolve(mode) {
          if (mode === "light" || mode === "dark") return mode;
          return window.matchMedia("(prefers-color-scheme: dark)").matches
            ? "dark" : "light";
        }
        var mode = localStorage.getItem("theme") || "system";
        document.documentElement.setAttribute("data-theme", resolve(mode));
      })();
    </script>
    <style>
      /* Registry-page additions on top of the site theme's tokens. */
      .registry-head {
        display: flex;
        align-items: baseline;
        justify-content: space-between;
        gap: 1rem;
        flex-wrap: wrap;
        margin-top: 1.5rem;
      }
      .module-grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(21rem, 1fr));
        gap: 1rem;
        margin: 1.25rem 0 2rem;
      }
      .module-card {
        background: var(--bg-raised);
        border: 1px solid var(--border);
        border-radius: 8px;
        padding: 0.9rem 1.1rem;
      }
      .module-card h2 {
        font-size: var(--step-1);
        font-family: var(--mono);
        margin: 0 0 0.5rem;
        display: flex;
        align-items: baseline;
        gap: 0.4rem;
      }
      .module-card details { margin-top: 0.4rem; }
      .module-card summary {
        cursor: pointer;
        color: var(--fg-muted);
        font-size: 0.9rem;
      }
      .module-card ul { list-style: none; margin: 0.4rem 0 0; padding: 0 0 0 0.6rem; }
      .module-card li { overflow-wrap: anywhere; font-size: 0.92rem; }
      .versions { font-size: 0.92rem; }
      .versions a { font-family: var(--mono); }
      .copy-btn {
        border: 0;
        background: none;
        color: var(--accent);
        cursor: pointer;
        font-size: 0.9em;
        padding: 0 0.15rem;
      }
      .copy-btn:hover { filter: brightness(0.8); }
      .badge-dev {
        font-family: var(--mono);
        font-size: 0.7rem;
        color: var(--fg-muted);
        border: 1px solid var(--border);
        border-radius: 4px;
        padding: 0 0.3rem;
      }
      del { color: var(--fg-muted); }

      /* Mermaid DAG panel */
      #mermaid-container {
        border: 1px solid var(--border);
        border-radius: 8px;
        background: var(--bg);
        position: relative;
        height: 85vh;
        min-height: 600px;
        width: 100%;
        overflow: hidden;
        margin-bottom: 2rem;
      }
      #dag-mermaid { width: 100%; height: 100%; }
      #mermaid-zoom-controls {
        position: absolute;
        top: 10px;
        left: 10px;
        z-index: 100;
        display: flex;
        flex-direction: column;
        gap: 5px;
      }
      #mermaid-zoom-controls button {
        border: 1px solid var(--border);
        background: var(--bg-raised);
        color: var(--fg);
        border-radius: 6px;
        width: 2rem;
        height: 2rem;
        cursor: pointer;
        font-size: 1rem;
        line-height: 1;
      }
      #mermaid-zoom-controls button:hover {
        border-color: var(--accent);
        color: var(--accent);
      }
      .mermaid { overflow-x: auto; max-width: 100%; }
      .mermaid svg { max-width: 100% !important; }
      .mermaid .inverted rect { fill: #333 !important; stroke: #000 !important; }
      .mermaid .inverted .label, .mermaid .inverted span { color: #fff !important; }
      .mermaid .leaf rect { fill: #28a745 !important; stroke: #1e7e34 !important; }
      .mermaid .leaf .label, .mermaid .leaf span { color: #fff !important; }
      [data-theme="dark"] .mermaid .inverted rect { fill: #eee !important; stroke: #fff !important; }
      [data-theme="dark"] .mermaid .inverted .label,
      [data-theme="dark"] .mermaid .inverted span { color: #111 !important; }
    </style>
    <script src="https://cdn.jsdelivr.net/npm/svg-pan-zoom@3.6.1/dist/svg-pan-zoom.min.js"></script>
</head>
<body>
    <header class="site-header">
      <nav class="container nav-row" aria-label="Site">
        <a class="brand" href="https://www.hdlfactory.com/">
          <img src="/logo/hf-logo.svg" alt="" width="40" height="40">
          <span>HDL Factory Home</span>
        </a>
        <ul class="menu">
          <li><a href="https://www.hdlfactory.com/">Home</a></li>
          <li><a href="https://www.hdlfactory.com/tags/">Tags</a></li>
          <li><a href="https://www.hdlfactory.com/index.xml">RSS</a></li>
          <li><a href="/bazel-registry/" aria-current="page">Bazel Registry</a></li>
          <li>
            <button id="theme-toggle" type="button" aria-label="Toggle color theme">
              <span id="theme-icon" aria-hidden="true"></span>
            </button>
          </li>
        </ul>
      </nav>
    </header>
    <script>
      (function () {
        var ICONS = { system: "◐", light: "☀", dark: "☾" };
        var btn = document.getElementById("theme-toggle");
        var icon = document.getElementById("theme-icon");
        function resolve(mode) {
          if (mode === "light" || mode === "dark") return mode;
          return window.matchMedia("(prefers-color-scheme: dark)").matches
            ? "dark" : "light";
        }
        function apply(mode) {
          document.documentElement.setAttribute("data-theme", resolve(mode));
          localStorage.setItem("theme", mode);
          icon.textContent = ICONS[mode];
          btn.title = "Theme: " + mode + " (click to change)";
        }
        btn.addEventListener("click", function () {
          var order = ["system", "light", "dark"];
          var cur = localStorage.getItem("theme") || "system";
          apply(order[(order.indexOf(cur) + 1) % order.length]);
        });
        apply(localStorage.getItem("theme") || "system");
      })();
    </script>

    <main class="container">
      <div class="registry-head">
        <h1 class="page-title"><a href="https://www.hdlfactory.com">My</a>
          <a href="https://bazel.build">Bazel</a> Registry</h1>
      </div>

      <div class="prose lead">
        <p>These modules are published in <a
        href="https://github.com/filmil/bazel-registry">my private bazel
        registry</a>. See the <a
        href="https://github.com/filmil/bazel-registry#usage">usage
        details</a> for how to configure bazel to use this additional
        registry.</p>

        <p>If you are wondering why all of this is built on bazel in the
        first place, I wrote up my reasons in <a
        href="https://hdlfactory.com/post/2024/04/27/why-do-i-bother-with-bazel/">Why
        do I bother with bazel?</a></p>

        <p>The bazel central registry is still available at <a
        href="https://bcr.bazel.build">https://bcr.bazel.build</a>.</p>
      </div>

      <div class="search">
        <input id="searchInput" type="search" placeholder="Search for modules&hellip;"
               aria-label="Search for modules">
      </div>

      <div class="module-grid" id="module-cards">
        {{range $module := .Modules}}
        <section class="module-card" id="card-{{sanitizeID $module.Name}}">
          <h2 class="card-title">
            {{$module.Name}}
            <a href="{{$module.Metadata.Homepage}}" title="Homepage">&#8599;</a>
          </h2>
          <div class="versions">
            <strong>Versions:</strong>
            {{if gt (len $module.Versions) 0}}
              {{$latest := index $module.Versions 0}}
              {{if isYanked $latest.Name $module.Metadata}}
                <span><del>{{$latest.Name}}</del></span>
              {{else}}
                <span title="{{ bazelDep $module.Name $latest.Name }}">
                  <a href="https://github.com/filmil/bazel-registry/tree/main/modules/{{$module.Name}}/{{$latest.Name}}">{{$latest.Name}}</a><button
                    class="copy-btn" title="Copy bazel_dep()"
                    onclick="copyToClipboard('{{ bazelDep $module.Name $latest.Name }}'); return false;"><svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><rect x="5.5" y="5.5" width="9" height="9" rx="1.5"/><path d="M10.5 3.5v-1a1.5 1.5 0 0 0-1.5-1.5h-6A1.5 1.5 0 0 0 1.5 2.5v6A1.5 1.5 0 0 0 3 10h1"/></svg></button>
                </span>
              {{end}}

              {{if gt (len $module.Versions) 1}}
                <details>
                  <summary>Older versions</summary>
                  <div>
                  {{range $i, $v := $module.Versions}}
                    {{if gt $i 0}}
                      {{if isYanked $v.Name $module.Metadata}}
                        <span><del>{{$v.Name}}</del></span>
                      {{else}}
                        <span title="{{ bazelDep $module.Name $v.Name }}">
                          <a href="https://github.com/filmil/bazel-registry/tree/main/modules/{{$module.Name}}/{{$v.Name}}">{{$v.Name}}</a><button
                            class="copy-btn" title="Copy bazel_dep()"
                            onclick="copyToClipboard('{{ bazelDep $module.Name $v.Name }}'); return false;"><svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><rect x="5.5" y="5.5" width="9" height="9" rx="1.5"/><path d="M10.5 3.5v-1a1.5 1.5 0 0 0-1.5-1.5h-6A1.5 1.5 0 0 0 1.5 2.5v6A1.5 1.5 0 0 0 3 10h1"/></svg></button>
                        </span>
                      {{end}}
                    {{end}}
                  {{end}}
                  </div>
                </details>
              {{end}}
            {{end}}
          </div>
          {{if gt (len $module.Versions) 0}}
            {{$latest := index $module.Versions 0}}
            {{if gt (len $latest.Dependencies) 0}}
              <details>
                <summary><strong>Dependencies (Latest)</strong></summary>
                <ul>
                {{range $dep := $latest.Dependencies}}
                  <li>
                    <code>{{$dep.Name}}</code> ({{$dep.Version}})
                    {{if $dep.DevDependency}}<span class="badge-dev">dev</span>{{end}}
                  </li>
                {{end}}
                </ul>
              </details>
            {{end}}
          {{end}}
          <details>
            <summary><strong>Links</strong></summary>
            <ul>
              <li><a href="{{$module.Metadata.Homepage}}">{{$module.Metadata.Homepage}}</a></li>
              <li>
                {{$repo := index $module.Metadata.Repo 0}}
                <a href="{{repoURL $repo}}">{{$repo}}</a>
              </li>
            </ul>
          </details>
        </section>
        {{end}}
      </div>

      <h2>Module Dependency DAG (Latest Versions)</h2>
      <div id="mermaid-container">
        <div id="mermaid-zoom-controls">
          <button type="button" title="Zoom in" onclick="panZoom.zoomIn()">+</button>
          <button type="button" title="Zoom out" onclick="panZoom.zoomOut()">&minus;</button>
          <button type="button" title="Reset" onclick="panZoom.reset()"><svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M2 6V2h4M14 10v4h-4M2 2l5 5M14 14l-5-5"/></svg></button>
        </div>
        <div class="mermaid" id="dag-mermaid">
          {{.Mermaid}}
        </div>
      </div>
    </main>

    <script type="module">
        import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.esm.min.mjs';
        import elkLayouts from 'https://cdn.jsdelivr.net/npm/@mermaid-js/layout-elk@0/dist/mermaid-layout-elk.esm.min.mjs';
        // The DAG source carries frontmatter asking for the ELK layout with
        // merged, orthogonally routed edges; that only takes effect on
        // mermaid v11 with the ELK layout engine registered (v10 silently
        // ignored it and fell back to per-edge dagre curves).
        mermaid.registerLayoutLoaders(elkLayouts);
        mermaid.initialize({
            startOnLoad: false,
            // Without this, mermaid re-wraps long label lines at its own
            // width cap, folding the pre-wrapped external-modules bar.
            markdownAutoWrap: false,
            layout: 'elk',
            elk: {
                mergeEdges: true,
                nodePlacementStrategy: 'LINEAR_SEGMENTS'
            },
            flowchart: {
                useMaxWidth: false,
                // mermaid wraps html labels at ~200px by default, which
                // would fold the pre-wrapped external-modules bar back into
                // a tower; the generator controls wrapping itself.
                wrappingWidth: 2400
            }
        });

        // Use a global variable to store the panZoom instance
        window.panZoom = null;

        async function initMermaid() {
            const container = document.getElementById('dag-mermaid');
            // trim(): the div's HTML indentation prefixes the source with
            // whitespace, and mermaid v11 only recognizes the YAML config
            // frontmatter when '---' starts at the first character.
            const { svg } = await mermaid.render('dag-svg', container.textContent.trim());
            container.innerHTML = svg;

            const svgElement = container.querySelector('svg');
            svgElement.removeAttribute('height');
            svgElement.removeAttribute('width');
            svgElement.style.width = '100%';
            svgElement.style.height = '100%';
            svgElement.style.maxWidth = '100%';

            window.panZoom = svgPanZoom(svgElement, {
                zoomEnabled: true,
                controlIconsEnabled: true,
                fit: true,
                center: true,
                minZoom: 0.01,
                maxZoom: 50,
                refreshRate: 'auto',
            });

            window.panZoom.resize();
            window.panZoom.fit();
            window.panZoom.center();

            window.addEventListener('resize', () => {
                window.panZoom.resize();
                window.panZoom.fit();
                window.panZoom.center();
            });
        }

        document.addEventListener('DOMContentLoaded', () => {
            initMermaid();
        });
    </script>
    <script>
        const searchInput = document.getElementById('searchInput');
        const moduleCards = document.querySelectorAll('.module-card');

        searchInput.addEventListener('input', (event) => {
            const filter = event.target.value.toLowerCase();
            moduleCards.forEach(card => {
                const title = card.querySelector('.card-title').textContent.toLowerCase();
                card.style.display = title.includes(filter) ? '' : 'none';
            });
        });

        function copyToClipboard(text) {
            navigator.clipboard.writeText(text).then(function() {
                /* clipboard successfully set */
            }, function() {
                alert('Failed to copy');
            });
        }
    </script>
    <footer class="site-footer">
      <div class="container">
        <p>&copy; 2025-present Filip Filmar. All rights reserved.</p>
        <p class="fineprint">This page was generated by an automated coding assistant.</p>
      </div>
    </footer>
</body>
</html>
`
