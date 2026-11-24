// SPDX-License-Identifier: Apache-2.0

// Command presubmit runs the presubmit.yml of changed module versions in
// CI, after the model of the Bazel Central Registry's presubmit, on one
// Linux host. Matrix platforms are ignored, and tasks for a named Windows
// or macOS platform are skipped. Top-level tasks run on the top-level
// matrix's Bazel versions, test module tasks on the test module's.
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
	cmd := exec.Command("git", "-C", repo, "diff", "-z", "--name-only", base+"...HEAD", "--", "modules/")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git diff %s...HEAD: %w", base, err)
	}
	var paths []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
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
	return bazelCmdEnv(dir, ver, repo, nil, args...)
}

func bazelCmdEnv(dir, ver, repo string, env map[string]string, args ...string) error {
	if len(args) == 0 {
		return nil
	}
	reg := []string{"--registry=file://" + repo, "--registry=" + bcr}
	full := append([]string{args[0]}, append(reg, args[1:]...)...)
	log.Printf("+ (cd %s && USE_BAZEL_VERSION=%s bazel %s)", dir, ver, strings.Join(full, " "))
	cmd := exec.Command("bazel", full...)
	cmd.Dir = dir
	cmd.Env = withEnv(env, "USE_BAZEL_VERSION="+ver)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func withEnv(env map[string]string, extra ...string) []string {
	out := os.Environ()
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return append(out, extra...)
}

// runTask runs one task in dir: its shell commands, then its build and
// test targets.
func runTask(dir, ver, repo, name string, t Task) error {
	if !t.linuxOnly() {
		log.Printf("== task %s: skipped, platform %s cannot run on this Linux host", name, t.Platform)
		return nil
	}
	log.Printf("== task %s", name)
	for _, c := range t.ShellCommands {
		log.Printf("+ (cd %s && %s)", dir, c)
		cmd := exec.Command("bash", "-c", c)
		cmd.Dir = dir
		cmd.Env = withEnv(t.Environment, "USE_BAZEL_VERSION="+ver)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("task %s: shell command %q: %w", name, c, err)
		}
	}
	if len(t.BuildTargets) > 0 {
		if err := bazelCmdEnv(dir, ver, repo, t.Environment, targetArgs("build", t.BuildFlags, t.BuildTargets)...); err != nil {
			return fmt.Errorf("task %s: build: %w", name, err)
		}
	}
	if len(t.TestTargets) > 0 {
		if err := bazelCmdEnv(dir, ver, repo, t.Environment, targetArgs("test", t.TestFlags, t.TestTargets)...); err != nil {
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
		makeWritable(work)
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
	// A real consumer has a root package, and rules_rust's crate_universe
	// fails without one, even in a dependency's extension call
	// (https://github.com/bazelbuild/rules_rust/issues/4385).
	if err := os.WriteFile(filepath.Join(probe, "BUILD.bazel"), nil, 0o644); err != nil {
		return err
	}
	// The top-level tasks build the module's own targets as a dependency,
	// on the top-level matrix's Bazel versions only.
	if contains(ps.topVersions(j.Bazel), j.Bazel) {
		for _, n := range sortedTasks(ps.Tasks) {
			if err := runTask(probe, j.Bazel, repo, n, ps.Tasks[n]); err != nil {
				return err
			}
		}
	}
	if ps.TestModule == nil {
		log.Printf("%s@%s has no bcr_test_module", j.Module, j.Version)
		return nil
	}
	if !contains(ps.testModuleVersions(j.Bazel), j.Bazel) {
		log.Printf("the test module of %s@%s does not run on Bazel %s", j.Module, j.Version, j.Bazel)
		return nil
	}
	// The test module lives inside the module's source, as BCR's presubmit
	// finds it: vendor the module and join module_path onto it.
	vendor := filepath.Join(work, "vendor")
	if err := bazelCmd(probe, j.Bazel, repo, "vendor", "--vendor_dir="+vendor, "--repo=@"+j.Module); err != nil {
		return fmt.Errorf("vendor @%s: %w", j.Module, err)
	}
	vd, err := vendoredDir(vendor, j.Module)
	if err != nil {
		return err
	}
	// Bazel writes a lockfile and convenience symlinks in the test module,
	// and vendored files can be read-only.
	makeWritable(vd)
	tm := filepath.Join(vd, ps.TestModule.ModulePath)
	if _, err := os.Stat(filepath.Join(tm, "MODULE.bazel")); err != nil {
		return fmt.Errorf("test module %s: %w", tm, err)
	}
	if err := stripRegistries(tm); err != nil {
		return err
	}
	ran = append(ran, tm)
	for _, n := range sortedTasks(ps.TestModule.Tasks) {
		if err := runTask(tm, j.Bazel, repo, n, ps.TestModule.Tasks[n]); err != nil {
			return err
		}
	}
	return nil
}

// makeWritable gives the owner write access to every directory and file
// under root. It does not follow symlinks, so nothing outside root changes.
func makeWritable(root string) {
	_ = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		_ = os.Chmod(p, fi.Mode().Perm()|0o200)
		return nil
	})
}
