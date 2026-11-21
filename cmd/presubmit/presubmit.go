// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Task is one task of a presubmit.yml: targets to build and to test, and
// the flags for each. Platform names are ignored: CI runs on one host.
type Task struct {
	Name         string   `yaml:"name"`
	BuildFlags   []string `yaml:"build_flags"`
	BuildTargets []string `yaml:"build_targets"`
	TestFlags    []string `yaml:"test_flags"`
	TestTargets  []string `yaml:"test_targets"`
}

// Matrix is the part of a presubmit.yml matrix that CI uses.
type Matrix struct {
	Bazel []string `yaml:"bazel"`
}

// TestModule is the bcr_test_module section of a presubmit.yml.
type TestModule struct {
	ModulePath string          `yaml:"module_path"`
	Matrix     Matrix          `yaml:"matrix"`
	Tasks      map[string]Task `yaml:"tasks"`
}

// Presubmit is a module version's presubmit.yml.
type Presubmit struct {
	Matrix     Matrix          `yaml:"matrix"`
	Tasks      map[string]Task `yaml:"tasks"`
	TestModule *TestModule     `yaml:"bcr_test_module"`
}

// Job is one CI job: a module version, run with one Bazel version.
type Job struct {
	Module  string `json:"module"`
	Version string `json:"version"`
	Bazel   string `json:"bazel"`
}

// readPresubmit reads modules/<module>/<version>/presubmit.yml under repo.
func readPresubmit(repo, module, version string) (*Presubmit, error) {
	p := filepath.Join(repo, "modules", module, version, "presubmit.yml")
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var ps Presubmit
	if err := yaml.Unmarshal(b, &ps); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return &ps, nil
}

// bazelVersions returns the Bazel versions a presubmit asks for: those of
// the test module's matrix, else those of the top-level matrix, else the
// default. A version written as a ${{ }} expression cannot be resolved
// here and is skipped.
func (ps *Presubmit) bazelVersions(def string) []string {
	var vs []string
	if ps.TestModule != nil {
		vs = ps.TestModule.Matrix.Bazel
	}
	if len(vs) == 0 {
		vs = ps.Matrix.Bazel
	}
	var out []string
	for _, v := range vs {
		if !strings.Contains(v, "${{") {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		out = []string{def}
	}
	return out
}

var versionDir = regexp.MustCompile(`^modules/([^/]+)/([^/]+)/`)

// changedVersions returns the module versions that the changed paths touch
// and that have a presubmit.yml in repo, sorted. A path outside a version
// directory (metadata.json, for example) touches no version.
func changedVersions(repo string, paths []string) [][2]string {
	seen := map[[2]string]bool{}
	for _, p := range paths {
		m := versionDir.FindStringSubmatch(p)
		if m == nil {
			continue
		}
		k := [2]string{m[1], m[2]}
		if seen[k] {
			continue
		}
		if _, err := os.Stat(filepath.Join(repo, "modules", m[1], m[2], "presubmit.yml")); err != nil {
			continue
		}
		seen[k] = true
	}
	out := make([][2]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}

// plan turns the changed paths into CI jobs: one per changed module
// version and Bazel version.
func plan(repo string, paths []string, defBazel string) ([]Job, error) {
	jobs := []Job{}
	for _, mv := range changedVersions(repo, paths) {
		ps, err := readPresubmit(repo, mv[0], mv[1])
		if err != nil {
			return nil, err
		}
		for _, b := range ps.bazelVersions(defBazel) {
			jobs = append(jobs, Job{Module: mv[0], Version: mv[1], Bazel: b})
		}
	}
	return jobs, nil
}

// sortedTasks returns a presubmit's tasks in name order, so runs repeat.
func sortedTasks(ts map[string]Task) []string {
	names := make([]string, 0, len(ts))
	for n := range ts {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
