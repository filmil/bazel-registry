"""dot_s8_candidate: the chip-harness flow for one candidate, as targets.

For a candidate named <c>, with the upstream defaults (27 taps, 10 ns):

  <c>_lint_test       verilator --lint-only
  <c>_rtl_sim_test    Icarus against the golden model
  <c>_formal_test     Yosys SAT equivalence with harness/ref.v at 1 tap
  <c>_design          design.v (copied, or printed by gen.py)
  <c>_synth           Yosys and ABC to IHP SG13G2: syn_area.v and .stat
  <c>_gate            the netlist, cell models and tb.v, built by Verilator
  <c>_gate_sim_test   the gate-level vectors on <c>_gate
  <c>_sta             OpenSTA timing and power: <c>/sta_area.json and .log
  <c>_sta_test        checks the slack and power in <c>/sta_area.json

These are the gates of upstream harness/evaluate.py, in its order. Each
step calls the evaluate.py function that does it; see stage.py.
`synth_tags` go on <c>_synth and every target after it, so slow
syntheses can be tagged "manual".

dot_s8_rejected_test is for the negative controls: it runs
evaluate.evaluate() up to formal and passes only when the candidate is
rejected at the given stage.
"""

load("@rules_cc//cc:cc_binary.bzl", "cc_binary")
load("@rules_python//python:py_test.bzl", "py_test")
load("@rules_verilator//verilator:defs.bzl", "verilator_cc_library")
load("@rules_verilog//verilog:defs.bzl", "verilog_library")

_EVALUATE = Label("//:harness/evaluate.py")
_HARNESS = Label("//:harness")
_PDK = Label("//:pdk")
_STAGE = Label("//bazel:stage")
_STAGE_PY = Label("//bazel:stage.py")
_IVERILOG = Label("@iverilog//:iverilog")
_IVL_BASE = Label("@iverilog//:ivl_base")
_VVP = Label("@iverilog//:vvp")
_VERILATOR = Label("@verilator//:verilator")
_YOSYS = Label("@yosys//:yosys")
_STA = Label("@opensta//:sta")

def _stage_run_impl(ctx):
    targets = ctx.attr.tools + ctx.attr.srcs
    args = [ctx.attr.step] + [
        ctx.expand_location(a, targets = targets)
        for a in ctx.attr.args
    ] + ["--out=" + f.path for f in ctx.outputs.outs]
    ctx.actions.run(
        executable = ctx.executable._stage,
        arguments = args,
        inputs = ctx.files.srcs,
        tools = [t[DefaultInfo].files_to_run for t in ctx.attr.tools],
        outputs = ctx.outputs.outs,
        mnemonic = "ChipHarness" + ctx.attr.step.capitalize().replace("_", ""),
        progress_message = "chip-harness %s %%{label}" % ctx.attr.step,
    )
    return [DefaultInfo(files = depset(ctx.outputs.outs))]

# Runs stage.py once, as a build action. The tools are built for the
# target configuration, as for the tests, so each tool is built once.
_stage_run = rule(
    implementation = _stage_run_impl,
    attrs = {
        "args": attr.string_list(),
        "outs": attr.output_list(mandatory = True),
        "srcs": attr.label_list(allow_files = True),
        "step": attr.string(mandatory = True),
        "tools": attr.label_list(allow_files = True, cfg = "target"),
        "_stage": attr.label(
            default = _STAGE,
            executable = True,
            cfg = "target",
        ),
    },
)

def _common_args(candidate, taps, period, path = "rootpath"):
    """Arguments of every step; path is rootpath in tests, execpath in actions."""
    return [
        "--evaluate=$(%s %s)" % (path, _EVALUATE),
        "--candidate=$(%s %s)" % (path, candidate),
        "--taps=%d" % taps,
        "--period=%s" % period,
    ]

def _tool_args(paths):
    return ["--%s=$(rootpath %s)" % (k, v) for k, v in paths]

_ALL_TOOLS = [
    ("iverilog", _IVERILOG),
    ("ivl-base", _IVL_BASE),
    ("vvp", _VVP),
    ("verilator", _VERILATOR),
    ("yosys", _YOSYS),
]

def _clog2(n):
    b = 0
    for _ in range(64):
        if (1 << b) >= n:
            break
        b += 1
    return b

def _stage_test(name, step, srcs, candidate, taps, period, tools, extra_args = [], **kwargs):
    py_test(
        name = name,
        srcs = [_STAGE_PY],
        main = _STAGE_PY,
        args = [step] + _common_args(candidate, taps, period) +
               _tool_args(tools) + extra_args,
        data = [_EVALUATE, _HARNESS, _PDK] + srcs + [v for _, v in tools],
        **kwargs
    )

def dot_s8_candidate(
        name,
        srcs,
        candidate,
        taps = 27,
        period = 10.0,
        synth_tags = [],
        **kwargs):
    """The flow of harness/evaluate.py for one candidate.

    Args:
      name: the candidate name, the prefix of every target.
      srcs: the candidate's files: design.v, or gen.py and what it imports.
      candidate: design.v or gen.py, a label in srcs.
      taps: N, the number of taps.
      period: the clock period in ns, for gate simulation and STA.
      synth_tags: tags for synthesis and every target after it.
      **kwargs: passed to every test, e.g. size.
    """
    yw = 16 + _clog2(taps)
    half = "%s" % (period / 2)
    if candidate not in srcs:
        srcs = srcs + [candidate]
    _stage_test(
        name = name + "_lint_test",
        step = "lint",
        srcs = srcs,
        candidate = candidate,
        taps = taps,
        period = period,
        tools = [("verilator", _VERILATOR)],
        **kwargs
    )
    _stage_test(
        name = name + "_rtl_sim_test",
        step = "rtl_sim",
        srcs = srcs,
        candidate = candidate,
        taps = taps,
        period = period,
        tools = [("iverilog", _IVERILOG), ("ivl-base", _IVL_BASE), ("vvp", _VVP)],
        **kwargs
    )
    _stage_test(
        name = name + "_formal_test",
        step = "formal",
        srcs = srcs,
        candidate = candidate,
        taps = taps,
        period = period,
        tools = [("yosys", _YOSYS)],
        **kwargs
    )

    _stage_run(
        name = name + "_design",
        step = "emit",
        srcs = [_EVALUATE, _HARNESS] + srcs,
        args = _common_args(candidate, taps, period, "execpath"),
        outs = [name + "/design.v"],
    )
    _stage_run(
        name = name + "_synth",
        step = "synth",
        srcs = [_EVALUATE, _HARNESS, _PDK, ":%s_design" % name] + srcs,
        tools = [_YOSYS],
        args = _common_args(candidate, taps, period, "execpath") + [
            "--yosys=$(execpath %s)" % _YOSYS,
            "--design=$(execpath :%s_design)" % name,
        ],
        outs = [
            name + "/syn_area.v",
            name + "/syn_area.stat",
            name + "/syn.ys",
            name + "/syn.log",
        ],
        tags = synth_tags,
    )

    # The netlist with the Liberty-derived cell models and tb.v, as
    # evaluate.gate_build() builds it: the same parameters and warning
    # waivers, -O1, and -O0 for the C++.
    verilog_library(
        name = name + "_gate_sv",
        srcs = [
            Label("//:harness/tb.v"),
            ":%s/syn_area.v" % name,
            Label("//:pdk/sg13g2_func.v"),
        ],
        top_module = "tb",
        tags = synth_tags,
    )
    verilator_cc_library(
        name = name + "_gate_verilated",
        module = ":%s_gate_sv" % name,
        timing = True,
        trace_mode = "vcd",
        copts = ["-O0"],
        vopts = [
            "--main",
            "--trace-underscore",
            "-Wno-fatal",
            "-Wno-WIDTH",
            "-Wno-PINMISSING",
            "-Wno-TIMESCALEMOD",
            "-GN=%d" % taps,
            "-GYW=%d" % yw,
            "-GHALF=%s" % half,
            "-O1",
        ],
        tags = synth_tags,
    )
    cc_binary(
        name = name + "_gate",
        srcs = [Label("//bazel:gate_main.cc")],
        deps = [":%s_gate_verilated" % name],
        tags = synth_tags,
    )
    _stage_test(
        name = name + "_gate_sim_test",
        step = "gate_sim",
        srcs = srcs + [":%s_gate" % name],
        candidate = candidate,
        taps = taps,
        period = period,
        tools = [],
        extra_args = ["--sim=$(rootpath :%s_gate)" % name],
        tags = synth_tags,
        **kwargs
    )
    _stage_run(
        name = name + "_sta",
        step = "sta",
        srcs = [_EVALUATE, _HARNESS, _PDK, ":%s/syn_area.v" % name] + srcs,
        tools = [_STA, ":%s_gate" % name],
        args = _common_args(candidate, taps, period, "execpath") + [
            "--sta=$(execpath %s)" % _STA,
            "--sim=$(execpath :%s_gate)" % name,
            "--netlist=$(execpath :%s/syn_area.v)" % name,
        ],
        outs = [
            name + "/sta_area.json",
            name + "/sta_area.log",
        ],
        tags = synth_tags,
    )
    _stage_test(
        name = name + "_sta_test",
        step = "check_sta",
        srcs = srcs + [":%s/sta_area.json" % name],
        candidate = candidate,
        taps = taps,
        period = period,
        tools = [],
        extra_args = ["--result=$(rootpath :%s/sta_area.json)" % name],
        tags = synth_tags,
        **kwargs
    )

def dot_s8_rejected_test(name, srcs, candidate, stage, taps = 27, period = 10.0, **kwargs):
    """Passes only when evaluate.evaluate() rejects the candidate at stage.

    Args:
      name: the test name.
      srcs: the candidate's files.
      candidate: design.v or gen.py, a label in srcs.
      stage: the stage that must reject it: lint, rtl_sim or formal.
      taps: N, the number of taps.
      period: the clock period in ns.
      **kwargs: passed to the test.
    """
    if candidate not in srcs:
        srcs = srcs + [candidate]
    _stage_test(
        name = name,
        step = "reject",
        srcs = srcs,
        candidate = candidate,
        taps = taps,
        period = period,
        tools = _ALL_TOOLS,
        extra_args = ["--reject-at=" + stage],
        **kwargs
    )
