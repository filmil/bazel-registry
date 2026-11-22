"""Runs one step of the chip-harness flow for one dot_s8 candidate.

Each step calls the function of upstream harness/evaluate.py that does it,
with the tools that Bazel built, so it runs the same commands, vectors and
pass rules as `python3 harness/evaluate.py <candidate>`. The steps:

  lint      verilator --lint-only, the command of evaluate.evaluate(),
            with --no-std added (see _tool_env).
  rtl_sim   evaluate.vectors() and evaluate.sim(): Icarus against the
            golden model.
  formal    evaluate.formal(): Yosys SAT equivalence with harness/ref.v.
  reject    evaluate.evaluate(stop_after="formal"), for the negative
            controls: passes only if the candidate is REJECTED@<stage>.
  emit      evaluate.emit_design(): design.v for synthesis.
  synth     evaluate.synth(): Yosys and ABC to IHP SG13G2 cells.
  gate_sim  the gate-level vectors of evaluate.evaluate(), run on a
            Verilator build of the netlist, and evaluate._verdict().
  sta       the activity run of evaluate.evaluate() (+dump), then
            evaluate.sta(): OpenSTA timing and power.
  check_sta checks that the sta step reported slack, and power from the
            gate-level VCD.

The test steps print the stage result as JSON and exit 0 only on a pass.
The build steps (emit, synth, sta) write their outputs and exit 0 only
on a pass.
"""

import argparse
import importlib.util
import json
import os
import shlex
import shutil
import sys
import tempfile

# Nothing may be written next to the sources, which are in the source tree
# or in an external repository: no __pycache__ for evaluate.py, golden.py,
# or the candidates/_shared modules that gen.py imports in a child python.
sys.dont_write_bytecode = True
os.environ["PYTHONDONTWRITEBYTECODE"] = "1"


def _abs(p):
    return os.path.abspath(p) if p else p


def _tool_env(a, root):
    """Puts the tools on PATH under the names evaluate.py runs them by."""
    bindir = tempfile.mkdtemp(prefix="tools-", dir=root)
    for name, path in [
        ("iverilog", a.iverilog),
        ("vvp", a.vvp),
        ("yosys", a.yosys),
    ]:
        if path:
            os.symlink(_abs(path), os.path.join(bindir, name))
    if a.verilator:
        # Only lint runs verilator from PATH. The Bazel-built verilator has
        # no VERILATOR_ROOT, so it cannot load include/verilated_std.sv and
        # its waivers, and every lint stops with "Cannot find
        # verilated_std.sv". --no-std leaves out the std:: package, as
        # rules_verilator does. The candidates do not use it.
        w = os.path.join(bindir, "verilator")
        with open(w, "w") as f:
            f.write('#!/bin/sh\nexec %s --no-std "$@"\n'
                    % shlex.quote(_abs(a.verilator)))
        os.chmod(w, 0o755)
    os.environ["PATH"] = bindir + os.pathsep + os.environ.get("PATH", "")
    if a.ivl_base:
        # The iverilog driver finds ivl, ivlpp and vvp.tgt in IVL_BASE, and
        # vvp finds system.vpi on IVERILOG_VPI_MODULE_PATH.
        os.environ["IVL_BASE"] = _abs(a.ivl_base)
        os.environ["IVERILOG_VPI_MODULE_PATH"] = _abs(a.ivl_base)
    if a.sta:
        os.environ["OPENSTA"] = _abs(a.sta)
    # Synthesis and formal both use the yosys on PATH.
    os.environ["YOSYS_SYN"] = "yosys"
    os.environ.pop("SYNTH_FLAGS", None)
    os.environ.pop("RESULT_TAG", None)


def _workdir_root():
    """Where the steps work: the test's TEST_TMPDIR, or for a build step a
    new temporary directory, which main() removes at the end. Never the
    source tree or the output tree: evaluate.evaluate() would use
    <ROOT>/work, and stage.py points ROOT here too."""
    if os.environ.get("TEST_TMPDIR"):
        return os.environ["TEST_TMPDIR"], False
    return tempfile.mkdtemp(prefix="chip-harness-"), True


def _load_evaluate(path):
    """Imports harness/evaluate.py, which imports golden.py next to it."""
    harness = os.path.dirname(_abs(path))
    sys.path.insert(0, harness)
    spec = importlib.util.spec_from_file_location("evaluate", _abs(path))
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def _report(stage, ok, info):
    print(json.dumps({"stage": stage, "pass": ok, "result": info}, indent=1))
    if not ok:
        print("FAIL: %s did not pass" % stage)
    return 0 if ok else 1


def main():
    root, temp = _workdir_root()
    try:
        return _main(root)
    finally:
        if temp:
            shutil.rmtree(root, ignore_errors=True)


def _main(root):
    ap = argparse.ArgumentParser()
    ap.add_argument("step")
    ap.add_argument("--evaluate", required=True, help="harness/evaluate.py")
    ap.add_argument("--candidate", required=True,
                    help="design.v or gen.py of the candidate")
    ap.add_argument("--taps", type=int, default=27)
    ap.add_argument("--period", type=float, default=10.0)
    ap.add_argument("--formal-taps", default="1")
    ap.add_argument("--reject-at", help="reject: the stage that must fail")
    ap.add_argument("--iverilog")
    ap.add_argument("--vvp")
    ap.add_argument("--ivl-base")
    ap.add_argument("--verilator")
    ap.add_argument("--yosys")
    ap.add_argument("--sta")
    ap.add_argument("--design", help="synth: design.v from emit")
    ap.add_argument("--netlist", help="sta: syn_area.v from synth")
    ap.add_argument("--sim", help="gate_sim, sta: the Verilator gate build")
    ap.add_argument("--result", help="check_sta: sta_area.json from sta")
    ap.add_argument("--out", action="append", default=[],
                    help="emit, synth, sta: output files, in step order")
    a = ap.parse_args()

    outs = [_abs(o) for o in a.out]
    _tool_env(a, root)
    ev = _load_evaluate(a.evaluate)
    ev.ROOT = root
    cand = os.path.dirname(_abs(a.candidate))
    param = not os.path.exists(os.path.join(cand, "gen.py"))
    n = a.taps
    wd = tempfile.mkdtemp(prefix=a.step + "-", dir=root)

    if a.step == "lint":
        design, param = ev.emit_design(cand, n, os.path.join(wd, "design.v"))
        # The lint command of evaluate.evaluate(), stage 1.
        g = f"-GN={n}" if param else ""
        rc, out, t = ev.run(
            f"verilator --lint-only -Wall -Wno-fatal -Wno-DECLFILENAME "
            f"-Wno-UNUSEDSIGNAL -Wno-UNUSEDPARAM {g} --top-module dot_s8 "
            f"{shlex.quote(design)}", wd, log="lint.log")
        errs = out.count("%Error")
        warns = out.count("%Warning")
        print(out)
        return _report("lint", rc == 0 and errs == 0,
                       {"errors": errs, "warnings": warns, "sec": t})

    if a.step == "rtl_sim":
        design, param = ev.emit_design(cand, n, os.path.join(wd, "design.v"))
        # The vectors of evaluate.evaluate(), stage 2.
        nvec = ev.vectors(n, wd, [("corner", 200, 1), ("uniform", 4000, 2),
                                  ("conv", 1024, 3)])
        ok, info = ev.sim(wd, [design], n, nvec, param, "rtl_sim")
        return _report("rtl_sim", ok, info)

    if a.step == "formal":
        res = []
        ok = True
        for k in (int(x) for x in a.formal_taps.split(",")):
            ok_k, info = ev.formal(cand, wd, k)
            res.append(info)
            if not ok_k:
                ok = False
                break
        return _report("formal", ok, res)

    if a.step == "reject":
        # evaluate.evaluate() runs lint, rtl_sim and formal in order and
        # stops at the first failure. It works in <ROOT>/work/<name>, and
        # ROOT is the work directory set above.
        r = ev.evaluate(cand, n=n, formal_taps=tuple(
            int(x) for x in a.formal_taps.split(",")), stop_after="formal")
        want = "REJECTED@" + a.reject_at
        print(json.dumps(r, indent=1))
        if r["status"] != want:
            print("FAIL: status is %s, not %s" % (r["status"], want))
            return 1
        print("PASS: %s" % want)
        return 0

    if a.step == "emit":
        ev.emit_design(cand, n, os.path.join(wd, "design.v"))
        shutil.copy(os.path.join(wd, "design.v"), outs[0])
        return 0

    if a.step == "synth":
        # Outputs: syn_area.v, syn_area.stat, syn.ys, syn.log.
        design = os.path.join(wd, "design.v")
        shutil.copy(_abs(a.design), design)
        ok, res = ev.synth(wd, design, param, n, ("area",))
        for name, dst in zip(["syn_area.v", "syn_area.stat", "syn.ys",
                              "syn.log"], outs):
            src = os.path.join(wd, name)
            if os.path.exists(src):
                shutil.copy(src, dst)
        print(json.dumps(res, indent=1))
        if not ok:
            print("FAIL: synth did not pass")
            log = os.path.join(wd, "syn.log")
            if os.path.exists(log):
                sys.stdout.write(open(log).read()[-2000:])
            return 1
        return 0

    if a.step == "gate_sim":
        # The gate-level vectors of evaluate.evaluate(), stage 5.
        nvec = ev.vectors(n, wd, [("corner", 200, 1), ("uniform", 4000, 2)])
        rc, out, t = ev.run(f"{shlex.quote(_abs(a.sim))} +nvec={nvec}", wd,
                            log="gate_area_run.log")
        print(out)
        ok, info = ev._verdict(rc, out, nvec, t)
        return _report("gate_sim", ok, info)

    if a.step == "sta":
        # Outputs: the result as JSON, then sta_area.log.
        # The activity run of evaluate.evaluate(): conv vectors, +dump.
        nvec = ev.vectors(n, wd, [("conv", 4096, 7)])
        rc, out, t = ev.run(f"{shlex.quote(_abs(a.sim))} +nvec={nvec} +dump",
                            wd,
                            log="gate_area_run.log")
        ok, info = ev._verdict(rc, out, nvec, t)
        vcd = None
        if ok:
            os.replace(os.path.join(wd, "dump.vcd"),
                       os.path.join(wd, "act_area.vcd"))
            vcd = "act_area.vcd"
        net = os.path.join(wd, "syn_area.v")
        shutil.copy(_abs(a.netlist), net)
        ok2, info2 = ev.sta(wd, "syn_area.v", "sta_area", a.period, vcd)
        res = {"activity_run": info, "sta": info2}
        if ok2:
            res["energy_per_op_pj"] = round(info2["power_mw"] * a.period, 2)
        json.dump(res, open(outs[0], "w"), indent=1)
        log = os.path.join(wd, "sta_area.log")
        if os.path.exists(log):
            shutil.copy(log, outs[1])
        else:
            open(outs[1], "w").close()
        print(json.dumps(res, indent=1))
        # As upstream: a failed activity run leaves the default activity
        # of evaluate.sta(); only an STA failure rejects the candidate.
        if not ok2:
            print("FAIL: sta did not pass")
            return 1
        return 0

    if a.step == "check_sta":
        res = json.load(open(_abs(a.result)))
        print(json.dumps(res, indent=1))
        s = res.get("sta", {})
        ok = all(k in s for k in ("worst_slack_ns", "min_period_ns",
                                  "power_mw")) and s["power_mw"] > 0
        # Stricter than upstream, which falls back to a default activity:
        # the power must come from the gate-level VCD of a clean run, so a
        # broken VCD dump or read_vcd fails this test.
        act = res.get("activity_run", {})
        vcd = (act.get("errors") == 0 and act.get("checked", 0) > 0
               and s.get("activity", "").startswith("gate-level VCD"))
        if not vcd:
            print("FAIL: the power does not come from the gate-level VCD")
        return _report("sta", ok and vcd, s)

    print("unknown step: " + a.step)
    return 2


if __name__ == "__main__":
    sys.exit(main())
