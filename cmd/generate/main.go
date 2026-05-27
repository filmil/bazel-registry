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
	)
	flag.StringVar(&modulesDir, "modules_dir", "", "The path to the modules directory.")
	flag.StringVar(&outputFile, "output", "", "The file name to output")
	flag.Parse()
	if modulesDir == "" {
		log.Printf("flag --modules_dir=... is required")
		os.Exit(1)
	}
	if outputFile == "" {
		log.Printf("flag --output=... is required")
		os.Exit(1)
	}

	if err := run(modulesDir, outputFile); err != nil {
		log.Printf("error: %v", err)
		os.Exit(1)
	}
}

func run(modulesDir, outputFile string) error {
	modules, err := findModules(modulesDir)
	if err != nil {
		log.Fatalf("failed to find modules: %v", err)
	}

	o, err := os.Create(outputFile)
	if err != nil {
		log.Printf("could not create: %v: %v", outputFile, err)
	}

	mermaid := buildMermaid(modules)

	if err := generateHTML(modules, mermaid, o); err != nil {
		log.Fatalf("failed to generate HTML: %v", err)
	}

	return nil
}

func buildMermaid(modules []Module) string {
	var sb strings.Builder
	sb.WriteString("graph TB\n")

	registryLatest := make(map[string]string)
	for _, m := range modules {
		if len(m.Versions) > 0 {
			registryLatest[m.Name] = m.Versions[0].Name
		}
	}

	nodes := make(map[string]bool)
	edges := make(map[string]bool)
	allNodes := make(map[string]bool)

	escape := func(s string) string {
		return strings.ReplaceAll(s, "\"", "\\\"")
	}

	for _, m := range modules {
		if len(m.Versions) == 0 {
			continue
		}
		latest := m.Versions[0]
		mID := sanitizeID(m.Name)
		if !nodes[mID] {
			sb.WriteString(fmt.Sprintf("    %s(\"%s<br/>%s\")\n", mID, escape(m.Name), escape(latest.Name)))
			nodes[mID] = true
			allNodes[mID] = true
		}

		hasInternalDeps := false
		for _, dep := range latest.Dependencies {
			depID := sanitizeID(dep.Name)
			if _, ok := registryLatest[dep.Name]; ok {
				hasInternalDeps = true
			}

			if !nodes[depID] {
				version := dep.Version
				if v, ok := registryLatest[dep.Name]; ok {
					version = v
				}
				sb.WriteString(fmt.Sprintf("    %s(\"%s<br/>%s\")\n", depID, escape(dep.Name), escape(version)))
				nodes[depID] = true
				allNodes[depID] = true
				if _, ok := registryLatest[dep.Name]; !ok {
					sb.WriteString(fmt.Sprintf("    class %s inverted\n", depID))
				}
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
		sb.WriteString(fmt.Sprintf("    click %s \"#card-%s\"\n", nID, nID))
	}

	sb.WriteString("    classDef inverted fill:#333,color:#fff\n")
	sb.WriteString("    classDef leaf fill:#28a745,color:#fff\n")
	return sb.String()
}

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9_]`)

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
    <title>Bazel Registry</title>
    <link
		href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0-alpha1/dist/css/bootstrap.min.css"
		rel="stylesheet"
	>
    <link
		href="https://cdn.jsdelivr.net/npm/bootstrap-icons@1.10.3/font/bootstrap-icons.css"
		rel="stylesheet"
	>
	<link rel="icon" href="hdlfactory.png" type="image/png">
	<!-- Google tag (gtag.js) -->
	<script async src="https://www.googletagmanager.com/gtag/js?id=G-BKGTF9GD1K"></script>
	<script>
	  window.dataLayer = window.dataLayer || [];
	  function gtag(){dataLayer.push(arguments);}
	  gtag('js', new Date());

	  gtag('config', 'G-BKGTF9GD1K');
	</script>
	<script>
      function getPreferredTheme() {
        const savedTheme = localStorage.getItem('theme');
        if (savedTheme) {
          return savedTheme;
        }
        return 'system';
      }

      function resolveTheme(theme) {
        if (theme === 'system') {
          const currentHour = new Date().getHours();
          return (currentHour >= 19 || currentHour < 7) ? 'dark' : 'light';
        }
        return theme;
      }

      function updateThemeIcon(theme) {
        const toggleBtn = document.getElementById('themeToggle');
        if (toggleBtn) {
          if (theme === 'light') {
            toggleBtn.innerHTML = '<i class="bi bi-sun"></i>';
          } else if (theme === 'dark') {
            toggleBtn.innerHTML = '<i class="bi bi-moon-stars"></i>';
          } else {
            toggleBtn.innerHTML = '<i class="bi bi-circle-half"></i>';
          }
        }
      }

      function setTheme(theme) {
        document.documentElement.setAttribute('data-bs-theme', resolveTheme(theme));
        localStorage.setItem('theme', theme);
        updateThemeIcon(theme);
      }

      setTheme(getPreferredTheme());

      document.addEventListener('DOMContentLoaded', () => {
        updateThemeIcon(getPreferredTheme());
      });

      function toggleTheme() {
        const currentTheme = getPreferredTheme();
        let newTheme;
        if (currentTheme === 'system') {
          newTheme = 'light';
        } else if (currentTheme === 'light') {
          newTheme = 'dark';
        } else {
          newTheme = 'system';
        }
        setTheme(newTheme);
      }
	</script>
	<style>
      [data-bs-theme="dark"] {
        --bs-body-color: #e9ecef;
        --bs-secondary-color: #adb5bd;
        --bs-tertiary-color: #dee2e6;
        --bs-link-color: #6ea8fe;
        --bs-link-hover-color: #9ec5fe;
      }
      [data-bs-theme="dark"] .card-title {
        color: #6ea8fe;
      }
      [data-bs-theme="dark"] .text-secondary {
        color: #ced4da !important;
      }
      [data-bs-theme="dark"] .text-muted {
        color: #adb5bd !important;
      }
      [data-bs-theme="dark"] code:not(pre code) {
        background-color: var(--bs-tertiary-bg);
        color: #e6edf3;
      }
      [data-bs-theme="dark"] blockquote {
        background-color: var(--bs-tertiary-bg);
        border-left-color: var(--bs-border-color);
        color: var(--bs-secondary-color);
      }
      /* Mermaid DAG styling */
      .mermaid {
        overflow-x: auto;
        max-width: 100%;
      }
      .mermaid svg {
        height: auto !important;
        max-width: 100% !important;
      }
      .mermaid .inverted rect {
        fill: #333 !important;
        stroke: #000 !important;
      }
      .mermaid .inverted .label, .mermaid .inverted span {
        color: #fff !important;
      }

      /* Leaf nodes styling */
      .mermaid .leaf rect {
        fill: #28a745 !important;
        stroke: #1e7e34 !important;
      }
      .mermaid .leaf .label, .mermaid .leaf span {
        color: #fff !important;
      }

      [data-bs-theme="dark"] .mermaid .inverted rect {
        fill: #eee !important;
        stroke: #fff !important;
      }
      [data-bs-theme="dark"] .mermaid .inverted .label, [data-bs-theme="dark"] .mermaid .inverted span {
        color: #111 !important;
      }
	</style>
</head>
<body>
    <div class="container">
		<div class="d-flex justify-content-between align-items-center mt-5">
			<h1 class="mb-0"><a href="https://www.hdlfactory.com">My</a> <a
			href="https://bazel.build">Bazel</a> Registry</h1>
			<button class="btn btn-outline-secondary" onclick="toggleTheme()" id="themeToggle" title="Toggle theme">
				<i class="bi bi-circle-half"></i>
			</button>
		</div>

		<p>These modules are published in <a
		href="https://github.com/filmil/bazel-registry">my private bazel
		registry</a> See the <a
		href="https://github.com/filmil/bazel-registry#usage">usage details</a>
		for how to configure bazel use this additional registry. </p>

		<p> The bazel central registry is still available at <a
		href="https://bcr.bazel.build"> https://bcr.bazel.build</a>. </p>

        <input class="form-control mb-4" id="searchInput" type="text" placeholder="Search for modules...">
        <div class="row" id="module-cards">
            {{range $module := .Modules}}
            <div class="col-md-4 mb-4 module-card" id="card-{{sanitizeID $module.Name}}">
                <div class="card">
                    <div class="card-body">
                        <h5 class="card-title">
							{{$module.Name}}
							<a href="{{$module.Metadata.Homepage}}"><i class="bi bi-link-45deg"></i></a>
						</h5>
                        <div class="card-text mb-2">
                            <strong>Versions:</strong>
                            {{if gt (len $module.Versions) 0}}
                                {{$latest := index $module.Versions 0}}
                                {{if isYanked $latest.Name $module.Metadata}}
                                    <span class="me-2"><del>{{$latest.Name}}</del></span>
                                {{else}}
                                    <span class="me-2" data-bs-toggle="tooltip" data-bs-placement="top" title="{{ bazelDep $module.Name $latest.Name }}">
                                        <a href="https://github.com/filmil/bazel-registry/tree/main/modules/{{$module.Name}}/{{$latest.Name}}">{{$latest.Name}}</a>
                                        <a href="#" onclick="copyToClipboard('{{ bazelDep $module.Name $latest.Name }}'); return false;">
                                            <i class="bi bi-clipboard"></i>
                                        </a>
                                    </span>
                                {{end}}

                                {{if gt (len $module.Versions) 1}}
                                    <details>
                                        <summary class="text-muted" style="cursor: pointer; font-size: 0.9em;">Older versions</summary>
                                        <div class="mt-2">
                                        {{range $i, $v := $module.Versions}}
                                            {{if gt $i 0}}
                                                {{if isYanked $v.Name $module.Metadata}}
                                                    <span class="me-2"><del>{{$v.Name}}</del></span>
                                                {{else}}
                                                    <span class="me-2" data-bs-toggle="tooltip" data-bs-placement="top" title="{{ bazelDep $module.Name $v.Name }}">
                                                        <a href="https://github.com/filmil/bazel-registry/tree/main/modules/{{$module.Name}}/{{$v.Name}}">{{$v.Name}}</a>
                                                        <a href="#" onclick="copyToClipboard('{{ bazelDep $module.Name $v.Name }}'); return false;">
                                                            <i class="bi bi-clipboard"></i>
                                                        </a>
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
                                <summary class="card-text mb-1"><strong>Dependencies (Latest):</strong></summary>
                                <ul class="list-unstyled mb-2 ms-2">
                                {{range $dep := $latest.Dependencies}}
                                    <li>
                                        <code>{{$dep.Name}}</code> ({{$dep.Version}})
                                        {{if $dep.DevDependency}}<span class="badge bg-secondary" style="font-size: 0.6em;">dev</span>{{end}}
                                    </li>
                                {{end}}
                                </ul>
                                </details>
                            {{end}}
                        {{end}}
                        <details>
                        <summary class="card-text mb-1"><strong>Links:</strong></summary>
                        <ul class="list-unstyled mb-2 ms-2">
                            <li><a href="{{$module.Metadata.Homepage}}">{{$module.Metadata.Homepage}}</a></li>
                            <li>
                                {{$repo := index $module.Metadata.Repo 0}}
                                <a href="{{repoURL $repo}}">{{$repo}}</a>
                            </li>
                        </ul>
                        </details>
                    </div>
                </div>
            </div>
            {{end}}
        </div>

		<div class="mt-5">
			<h3>Module Dependency DAG (Latest Versions)</h3>
			<div class="mermaid">
				{{.Mermaid}}
			</div>
		</div>
    </div>
    <script src="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0-alpha1/dist/js/bootstrap.bundle.min.js"></script>
	<script type="module">
		import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs';
		mermaid.initialize({ startOnLoad: true });
	</script>
    <script>
        const searchInput = document.getElementById('searchInput');
        const moduleCards = document.querySelectorAll('.module-card');

        searchInput.addEventListener('keyup', (event) => {
            const filter = event.target.value.toLowerCase();
            moduleCards.forEach(card => {
                const title = card.querySelector('.card-title').textContent.toLowerCase();
                if (title.includes(filter)) {
                    card.style.display = '';
                } else {
                    card.style.display = 'none';
                }
            });
        });

        const tooltipTriggerList = document.querySelectorAll('[data-bs-toggle="tooltip"]');
        const tooltipList = [...tooltipTriggerList].map(tooltipTriggerEl => new bootstrap.Tooltip(tooltipTriggerEl));

        function copyToClipboard(text) {
            navigator.clipboard.writeText(text).then(function() {
                /* clipboard successfully set */
            }, function() {
                /* clipboard write failed */
                alert('Failed to copy');
            });
        }
    </script>
    <footer class="text-center mt-4 py-3">
        <p>&copy; 2025-present Filip Filmar. All rights reserved.</p>
        <p><small>This page was generated by an automated coding assistant.</small></p>
    </footer>
</body>
</html>
`
