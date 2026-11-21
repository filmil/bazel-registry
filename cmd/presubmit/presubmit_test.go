// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const withTestModule = `matrix:
  platform: [debian13]
  bazel: [7.x]
tasks:
  verify_targets:
    name: Verify build targets
    platform: ${{ platform }}
    bazel: ${{ bazel }}
    build_targets:
      - "@a//..."
bcr_test_module:
  module_path: test_module
  matrix:
    platform: [debian13, ubuntu2404]
    bazel: [8.x, 9.x]
  tasks:
    run_test_module:
      name: Run test module
      platform: ${{ platform }}
      bazel: ${{ bazel }}
      build_flags: ["--cxxopt=-std=c++17"]
      build_targets: ["//..."]
      test_targets: ["//...", "-//slow/..."]
`

const topOnly = `matrix:
  bazel: ["${{ bazel }}"]
tasks:
  verify_targets:
    build_targets: ["@b//..."]
`

func repo(t *testing.T) string {
	r := t.TempDir()
	write(t, filepath.Join(r, "modules/a/1.0/presubmit.yml"), withTestModule)
	write(t, filepath.Join(r, "modules/a/1.0/source.json"), "{}")
	write(t, filepath.Join(r, "modules/a/0.9/source.json"), "{}") // no presubmit.yml
	write(t, filepath.Join(r, "modules/a/metadata.json"), "{}")
	write(t, filepath.Join(r, "modules/b/2.0/presubmit.yml"), topOnly)
	return r
}

func TestPresubmitParse(t *testing.T) {
	r := repo(t)
	ps, err := readPresubmit(r, "a", "1.0")
	if err != nil {
		t.Fatal(err)
	}
	if ps.TestModule == nil || ps.TestModule.ModulePath != "test_module" {
		t.Fatalf("test module = %+v", ps.TestModule)
	}
	task := ps.TestModule.Tasks["run_test_module"]
	if !reflect.DeepEqual(task.TestTargets, []string{"//...", "-//slow/..."}) {
		t.Errorf("test targets = %v", task.TestTargets)
	}
	if !reflect.DeepEqual(task.BuildFlags, []string{"--cxxopt=-std=c++17"}) {
		t.Errorf("build flags = %v", task.BuildFlags)
	}
	if got := ps.Tasks["verify_targets"].BuildTargets; !reflect.DeepEqual(got, []string{"@a//..."}) {
		t.Errorf("verify targets = %v", got)
	}
}

func TestBazelVersions(t *testing.T) {
	r := repo(t)
	a, _ := readPresubmit(r, "a", "1.0")
	if got := a.bazelVersions("8.x"); !reflect.DeepEqual(got, []string{"7.x", "8.x", "9.x"}) {
		t.Errorf("a: %v, want the top-level and the test module's matrices", got)
	}
	if got := a.topVersions("8.x"); !reflect.DeepEqual(got, []string{"7.x"}) {
		t.Errorf("a top: %v", got)
	}
	if got := a.testModuleVersions("8.x"); !reflect.DeepEqual(got, []string{"8.x", "9.x"}) {
		t.Errorf("a test module: %v", got)
	}
	b, _ := readPresubmit(r, "b", "2.0")
	if got := b.bazelVersions("8.x"); !reflect.DeepEqual(got, []string{"8.x"}) {
		t.Errorf("b: %v, want the default for an unresolvable expression", got)
	}
	empty := &Presubmit{}
	if got := empty.bazelVersions("9.x"); !reflect.DeepEqual(got, []string{"9.x"}) {
		t.Errorf("empty: %v", got)
	}
}

func TestPlan(t *testing.T) {
	r := repo(t)
	paths := []string{
		"modules/a/1.0/source.json",
		"modules/a/1.0/overlay/BUILD.bazel", // same version: one entry
		"modules/a/0.9/source.json",         // no presubmit.yml: skipped
		"modules/a/metadata.json",           // not a version: skipped
		"modules/b/2.0/presubmit.yml",
		"modules/c/3.0/source.json", // deleted version: skipped
		"README.md",
	}
	jobs, err := plan(r, paths, "8.x")
	if err != nil {
		t.Fatal(err)
	}
	want := []Job{
		{Module: "a", Version: "1.0", Bazel: "7.x"},
		{Module: "a", Version: "1.0", Bazel: "8.x"},
		{Module: "a", Version: "1.0", Bazel: "9.x"},
		{Module: "b", Version: "2.0", Bazel: "8.x"},
	}
	if !reflect.DeepEqual(jobs, want) {
		t.Errorf("plan = %+v\nwant %+v", jobs, want)
	}
	none, err := plan(r, []string{"README.md"}, "8.x")
	if err != nil {
		t.Fatal(err)
	}
	if none == nil || len(none) != 0 {
		t.Errorf("no changes: %#v, want an empty, non-nil list (it prints as [])", none)
	}
}

func TestPlanBadYAML(t *testing.T) {
	r := t.TempDir()
	write(t, filepath.Join(r, "modules/x/1/presubmit.yml"), "tasks: [unclosed")
	if _, err := plan(r, []string{"modules/x/1/presubmit.yml"}, "8.x"); err == nil {
		t.Error("bad YAML: no error")
	}
}

func TestTargetArgs(t *testing.T) {
	got := targetArgs("test", []string{"--config=ci"}, []string{"//...", "-//slow/..."})
	if want := []string{"test", "--config=ci", "--", "//...", "-//slow/..."}; !reflect.DeepEqual(got, want) {
		t.Errorf("plain: %v", got)
	}
	// bazelci.py lets a target list start with "--"; it must not double.
	got = targetArgs("build", nil, []string{"--", "//...", "-//slow/..."})
	if want := []string{"build", "--", "//...", "-//slow/..."}; !reflect.DeepEqual(got, want) {
		t.Errorf("leading --: %v", got)
	}
}

func TestLinuxOnly(t *testing.T) {
	for p, want := range map[string]bool{
		"":                true,
		"${{ platform }}": true,
		"ubuntu2404":      true,
		"debian13":        true,
		"windows":         false,
		"macos_arm64":     false,
	} {
		if got := (Task{Platform: p}).linuxOnly(); got != want {
			t.Errorf("platform %q: %v, want %v", p, got, want)
		}
	}
}

func TestVendoredDir(t *testing.T) {
	for _, name := range []string{"m+", "m~", "m~1.2.3"} {
		v := t.TempDir()
		if err := os.MkdirAll(filepath.Join(v, name), 0o755); err != nil {
			t.Fatal(err)
		}
		// A different module with a common prefix must not match.
		if err := os.MkdirAll(filepath.Join(v, "mm+"), 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := vendoredDir(v, "m")
		if err != nil || got != filepath.Join(v, name) {
			t.Errorf("%s: %q, %v", name, got, err)
		}
	}
	if _, err := vendoredDir(t.TempDir(), "m"); err == nil {
		t.Error("missing: no error")
	}
}

func TestStripRegistries(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, ".bazelrc"), "common --registry=https://example.test/\nbuild --cxxopt=-std=c++20\ncommon --registry=https://bcr.bazel.build\n")
	if err := stripRegistries(d); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(d, ".bazelrc"))
	if got := string(b); got != "build --cxxopt=-std=c++20\n" {
		t.Errorf(".bazelrc after: %q", got)
	}
	if err := stripRegistries(t.TempDir()); err != nil {
		t.Errorf("no .bazelrc: %v", err)
	}
}
