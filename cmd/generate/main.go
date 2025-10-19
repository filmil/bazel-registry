package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var modulesDir = flag.String("modules_dir", "", "The path to the modules directory.")

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
	flag.Parse()
	if *modulesDir == "" {
		log.Fatalf("-modules_dir is required")
	}

	modules, err := findModules(*modulesDir)
	if err != nil {
		log.Fatalf("failed to find modules: %v", err)
	}

	if err := generateHTML(modules); err != nil {
		log.Fatalf("failed to generate HTML: %v", err)
	}
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

func generateHTML(modules []Module) error {
	tmpl, err := template.New("index").Funcs(template.FuncMap{
		"isURL": func(s string) bool {
			return strings.HasPrefix(s, "http")
		},
	}).Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse HTML template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, modules); err != nil {
		return fmt.Errorf("failed to execute HTML template: %w", err)
	}

	outputDir := os.Getenv("BUILD_WORKING_DIRECTORY")
	if outputDir == "" {
		outputDir = "."
	}
	outputPath := filepath.Join(outputDir, outputFile)

	if err := ioutil.WriteFile(outputPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	fmt.Printf("Generated %s successfully.\n", outputPath)
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
