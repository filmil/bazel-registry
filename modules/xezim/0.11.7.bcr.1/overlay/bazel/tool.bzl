"""Builds xezim with its own Rust and clang toolchains, in opt mode."""

# The toolchains that the xezim_toolchains extension makes.
_TOOLCHAINS = [
    str(Label("@xezim_rust__x86_64-unknown-linux-gnu__stable//:all")),
    # Only the linux-x86_64 toolchain has the sysroot.
    str(Label("@xezim_llvm//:cc-toolchain-x86_64-linux")),
]

# The consumer's C and link flags are for the consumer's compiler, not for
# this clang, so they are cleared.
_CLEARED = [
    "//command_line_option:conlyopt",
    "//command_line_option:copt",
    "//command_line_option:cxxopt",
    "//command_line_option:features",
    "//command_line_option:host_conlyopt",
    "//command_line_option:host_copt",
    "//command_line_option:host_cxxopt",
    "//command_line_option:host_linkopt",
    "//command_line_option:linkopt",
    "//command_line_option:per_file_copt",
]

def _transition_impl(settings, _attr):
    out = {o: [] for o in _CLEARED}
    out["//command_line_option:compilation_mode"] = "opt"
    out["//command_line_option:extra_toolchains"] = _TOOLCHAINS + settings["//command_line_option:extra_toolchains"]
    return out

_xezim_transition = transition(
    implementation = _transition_impl,
    inputs = ["//command_line_option:extra_toolchains"],
    outputs = _CLEARED + [
        "//command_line_option:compilation_mode",
        "//command_line_option:extra_toolchains",
    ],
)

def _xezim_tool_impl(ctx):
    src = ctx.attr.binary[0][DefaultInfo]
    exe = ctx.actions.declare_file(ctx.label.name)
    ctx.actions.symlink(output = exe, target_file = src.files_to_run.executable, is_executable = True)
    return [DefaultInfo(
        executable = exe,
        files = depset([exe]),
        runfiles = ctx.runfiles(files = [exe]).merge(src.default_runfiles),
    )]

xezim_tool = rule(
    implementation = _xezim_tool_impl,
    attrs = {
        "binary": attr.label(cfg = _xezim_transition, executable = True, mandatory = True),
    },
    executable = True,
    doc = "The xezim binary, built with Rust 1.100 and clang in opt mode.",
)
