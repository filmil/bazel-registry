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
	Homepage string   `json:"homepage"`
	Repo     []string `json:"repository"`
	Versions []string `json:"versions"`
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
</head>
<body>
    <div class="container">
        <h1 class="mt-5">Bzl Bazel Registry</h1>
        <table class="table table-striped mt-4">
            <thead>
                <tr>
                    <th>Module</th>
                    <th>Versions</th>
                    <th>Homepage</th>
                    <th>Repository</th>
                </tr>
            </thead>
            <tbody>
                {{range $module := .}}
                <tr>
                    <td>{{$module.Name}}</td>
                    <td>
                        {{range $module.Versions}}
                        <a href="modules/{{$module.Name}}/{{.Name}}">{{.Name}}</a><br>
                        {{end}}
                    </td>
                    <td><a href="{{$module.Metadata.Homepage}}">{{$module.Metadata.Homepage}}</a></td>
                    <td>
                        {{$repo := index $module.Metadata.Repo 0}}
                        {{if isURL $repo}}
                            <a href="{{$repo}}">{{$repo}}</a>
                        {{else}}
                            {{$repo}}
                        {{end}}
                    </td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
</body>
</html>
`
