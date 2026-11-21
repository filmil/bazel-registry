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

// Task is one task of a presubmit.yml: commands to run, then targets to
// build and to test, with their flags and environment.
type Task struct {
	Name          string            `yaml:"name"`
	Platform      string            `yaml:"platform"`
	Environment   map[string]string `yaml:"environment"`
	ShellCommands []string          `yaml:"shell_commands"`
	BuildFlags    []string          `yaml:"build_flags"`
	BuildTargets  []string          `yaml:"build_targets"`
	TestFlags     []string          `yaml:"test_flags"`
	TestTargets   []string          `yaml:"test_targets"`
}

// linuxOnly reports whether a task can run on a Linux host. A platform
// given as a ${{ }} expression comes from the matrix, whose platforms CI
// ignores; a named Windows or macOS platform cannot run here.
func (t Task) linuxOnly() bool {
	p := strings.ToLower(t.Platform)
	return !strings.Contains(p, "windows") && !strings.Contains(p, "macos")
}

// targetArgs returns the command line for targets: the flags, then "--",
// then the targets. A target list that already starts with "--", as
// bazelci.py allows, keeps its own.
func targetArgs(verb string, flags, targets []string) []string {
	args := append([]string{verb}, flags...)
	if len(targets) == 0 || targets[0] != "--" {
		args = append(args, "--")
	}
	return append(args, targets...)
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

// concrete drops versions written as ${{ }} expressions, which cannot be
// resolved here, and falls back to def when none is left.
func concrete(vs []string, def string) []string {
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

// topVersions returns the Bazel versions of the top-level tasks.
func (ps *Presubmit) topVersions(def string) []string {
	return concrete(ps.Matrix.Bazel, def)
}

// testModuleVersions returns the Bazel versions of the test module's
// tasks; nil when there is no test module.
func (ps *Presubmit) testModuleVersions(def string) []string {
	if ps.TestModule == nil {
		return nil
	}
	return concrete(ps.TestModule.Matrix.Bazel, def)
}

// bazelVersions returns every Bazel version a presubmit asks for: those of
// the top-level matrix and of the test module's matrix, without repeats.
func (ps *Presubmit) bazelVersions(def string) []string {
	var out []string
	seen := map[string]bool{}
	vs := ps.topVersions(def)
	if len(ps.Tasks) == 0 {
		vs = nil
	}
	for _, v := range append(vs, ps.testModuleVersions(def)...) {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		out = []string{def}
	}
	return out
}

func contains(vs []string, v string) bool {
	for _, x := range vs {
		if x == v {
			return true
		}
	}
	return false
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

// vendoredDir finds a module's directory under a bazel vendor dir: <name>+
// on Bazel 8 and later, <name>~ or <name>~<version> on Bazel 7.
func vendoredDir(vendor, module string) (string, error) {
	es, err := os.ReadDir(vendor)
	if err != nil {
		return "", err
	}
	for _, e := range es {
		n := e.Name()
		if e.IsDir() && (n == module+"+" || n == module+"~" || strings.HasPrefix(n, module+"~")) {
			return filepath.Join(vendor, n), nil
		}
	}
	return "", fmt.Errorf("no vendored directory for %s in %s", module, vendor)
}

// stripRegistries removes --registry lines from a .bazelrc in dir, if any.
// Registries in a .bazelrc come before command-line ones and would win, so
// the test module would fetch from somewhere other than this checkout.
func stripRegistries(dir string) error {
	p := filepath.Join(dir, ".bazelrc")
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var keep []string
	for _, l := range strings.Split(string(b), "\n") {
		if !strings.Contains(l, "--registry") {
			keep = append(keep, l)
		}
	}
	return os.WriteFile(p, []byte(strings.Join(keep, "\n")), 0o644)
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
