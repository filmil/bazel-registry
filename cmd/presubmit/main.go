// SPDX-License-Identifier: Apache-2.0

// Command presubmit runs the presubmit.yml of changed module versions in
// CI, the way the Bazel Central Registry's presubmit does, on one host.
//
// plan prints, as a JSON list for a GitHub Actions matrix, one job per
// module version that the pull request changes and per Bazel version its
// presubmit.yml asks for:
//
//	presubmit plan --repo <checkout> --base <git ref>
//
// run runs one job. It fetches the module from this registry (the local
// checkout first, then the Bazel Central Registry) in a scratch
// workspace, builds the top-level tasks' targets there, vendors the
// module, and runs the bcr_test_module tasks in the vendored test module:
//
//	presubmit run --repo <checkout> --module <name> --version <version> --bazel <version>
//
// It calls `bazel`, which must be bazelisk, so that USE_BAZEL_VERSION picks
// the Bazel version.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const bcr = "https://bcr.bazel.build"

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		log.Fatal("usage: presubmit plan|run [flags]")
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	repo := fs.String("repo", "", "absolute path of the registry checkout")
	base := fs.String("base", "origin/main", "plan: the git ref the pull request merges into")
	module := fs.String("module", "", "run: module name")
	version := fs.String("version", "", "run: module version")
	bazel := fs.String("bazel", "8.x", "run: Bazel version; plan: the default when presubmit.yml names none")
	if err := fs.Parse(os.Args[2:]); err != nil {
		log.Fatal(err)
	}
	if !filepath.IsAbs(*repo) {
		log.Fatalf("--repo must be an absolute path, got %q", *repo)
	}
	switch os.Args[1] {
	case "plan":
		if err := runPlan(*repo, *base, *bazel); err != nil {
			log.Fatal(err)
		}
	case "run":
		if *module == "" || *version == "" {
			log.Fatal("run needs --module and --version")
		}
		if err := runJob(*repo, Job{Module: *module, Version: *version, Bazel: *bazel}); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf("unknown subcommand %q", os.Args[1])
	}
}

func runPlan(repo, base, defBazel string) error {
	cmd := exec.Command("git", "-C", repo, "diff", "--name-only", base+"...HEAD", "--", "modules/")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git diff %s...HEAD: %w", base, err)
	}
	paths := strings.Fields(string(out))
	jobs, err := plan(repo, paths, defBazel)
	if err != nil {
		return err
	}
	b, err := json.Marshal(jobs)
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

// bazelCmd runs bazel in dir with the registry flags; the Bazel version
// comes from USE_BAZEL_VERSION.
func bazelCmd(dir, ver, repo string, args ...string) error {
	if len(args) == 0 {
		return nil
	}
	reg := []string{"--registry=file://" + repo, "--registry=" + bcr}
	full := append([]string{args[0]}, append(reg, args[1:]...)...)
	log.Printf("+ (cd %s && USE_BAZEL_VERSION=%s bazel %s)", dir, ver, strings.Join(full, " "))
	cmd := exec.Command("bazel", full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "USE_BAZEL_VERSION="+ver)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runTask builds and tests one task's targets in dir.
func runTask(dir, ver, repo, name string, t Task) error {
	log.Printf("== task %s", name)
	if len(t.BuildTargets) > 0 {
		args := append([]string{"build"}, t.BuildFlags...)
		args = append(append(args, "--"), t.BuildTargets...)
		if err := bazelCmd(dir, ver, repo, args...); err != nil {
			return fmt.Errorf("task %s: build: %w", name, err)
		}
	}
	if len(t.TestTargets) > 0 {
		args := append([]string{"test"}, t.TestFlags...)
		args = append(append(args, "--"), t.TestTargets...)
		if err := bazelCmd(dir, ver, repo, args...); err != nil {
			return fmt.Errorf("task %s: test: %w", name, err)
		}
	}
	return nil
}

func runJob(repo string, j Job) error {
	ps, err := readPresubmit(repo, j.Module, j.Version)
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "presubmit-")
	if err != nil {
		return err
	}
	// Every directory where this job ran bazel gets a server of its own.
	// Stop them and remove the scratch directory at the end, so a local
	// run leaves nothing behind. Vendored files can be read-only.
	var ran []string
	defer func() {
		for _, d := range ran {
			c := exec.Command("bazel", "shutdown")
			c.Dir = d
			c.Env = append(os.Environ(), "USE_BAZEL_VERSION="+j.Bazel)
			_ = c.Run()
		}
		_ = filepath.Walk(work, func(p string, _ os.FileInfo, _ error) error { _ = os.Chmod(p, 0o755); return nil })
		_ = os.RemoveAll(work)
	}()
	probe := filepath.Join(work, "probe")
	ran = append(ran, probe)
	if err := os.MkdirAll(probe, 0o755); err != nil {
		return err
	}
	mod := fmt.Sprintf("module(name = \"presubmit_probe\")\n\nbazel_dep(name = %q, version = %q)\n", j.Module, j.Version)
	if err := os.WriteFile(filepath.Join(probe, "MODULE.bazel"), []byte(mod), 0o644); err != nil {
		return err
	}
	// The top-level tasks build the module's own targets as a dependency.
	for _, n := range sortedTasks(ps.Tasks) {
		if err := runTask(probe, j.Bazel, repo, n, ps.Tasks[n]); err != nil {
			return err
		}
	}
	if ps.TestModule == nil {
		log.Printf("%s@%s has no bcr_test_module", j.Module, j.Version)
		return nil
	}
	// The test module lives inside the module's source, as BCR's presubmit
	// finds it: vendor the module and join module_path onto it.
	vendor := filepath.Join(work, "vendor")
	if err := bazelCmd(probe, j.Bazel, repo, "vendor", "--vendor_dir="+vendor, "--repo=@"+j.Module); err != nil {
		return fmt.Errorf("vendor @%s: %w", j.Module, err)
	}
	tm := filepath.Join(vendor, j.Module+"+", ps.TestModule.ModulePath)
	if _, err := os.Stat(filepath.Join(tm, "MODULE.bazel")); err != nil {
		return fmt.Errorf("test module %s: %w", tm, err)
	}
	ran = append(ran, tm)
	for _, n := range sortedTasks(ps.TestModule.Tasks) {
		if err := runTask(tm, j.Bazel, repo, n, ps.TestModule.Tasks[n]); err != nil {
			return err
		}
	}
	return nil
}
