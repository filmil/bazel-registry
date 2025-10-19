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
	"sort"
	"strings"
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
	Name       string
	ModuleFile string
	SourceFile string
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

	if err := generateHTML(modules, o); err != nil {
		log.Fatalf("failed to generate HTML: %v", err)
	}

	return nil
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

		versions = append(versions, Version{
			Name:       versionDir.Name(),
			ModuleFile: string(moduleFileContent),
			SourceFile: string(sourceFileContent),
		})
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Name > versions[j].Name
	})

	return versions, nil
}

func generateHTML(modules []Module, w io.WriteCloser) error {
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
	}).Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse HTML template: %w", err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, modules); err != nil {
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
</head>
<body>
    <div class="container">
		<h1 class="mt-5"><a href="https://www.hdlfactory.com">My</a> <a
		href="https://bazel.build">Bazel</a> Registry</h1>

		<p>These modules are published in <a
		href="https://github.com/filmil/bazel-registry">my private bazel
		registry</a> See the <a
		href="https://github.com/filmil/bazel-registry#usage">usage details</a>
		for how to configure bazel use this additional registry. </p>

		<p> The bazel central registry is still available at <a
		href="https://bcr.bazel.build"> https://bcr.bazel.build</a>. </p>

        <input class="form-control mb-4" id="searchInput" type="text" placeholder="Search for modules...">
        <div class="row" id="module-cards">
            {{range $module := .}}
            <div class="col-md-4 mb-4 module-card">
                <div class="card">
                    <div class="card-body">
                        <h5 class="card-title">
							{{$module.Name}}
							<a href="{{$module.Metadata.Homepage}}"><i class="bi bi-link-45deg"></i></a>
						</h5>
                        <p class="card-text">
                            <strong>Versions:</strong>
                            {{range $i, $v := $module.Versions}}
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
                        </p>
                        <p class="card-text"><a href="{{$module.Metadata.Homepage}}">{{$module.Metadata.Homepage}}</a></p>
                        <p class="card-text">
                            {{$repo := index $module.Metadata.Repo 0}}
							<a href="{{repoURL $repo}}">{{$repo}}</a>
                        </p>
                    </div>
                </div>
            </div>
            {{end}}
        </div>
    </div>
    <script src="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0-alpha1/dist/js/bootstrap.bundle.min.js"></script>
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
