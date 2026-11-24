"""Builds xezim with Rust 1.100 and in opt mode, whatever the build's own mode."""

# The toolchains of the proxy repository that the xezim_rust extension makes.
_TOOLCHAINS = str(Label("@xezim_rust__x86_64-unknown-linux-gnu__stable//:all"))

def _transition_impl(settings, _attr):
    return {
        "//command_line_option:compilation_mode": "opt",
        "//command_line_option:extra_toolchains": [_TOOLCHAINS] + settings["//command_line_option:extra_toolchains"],
    }

_xezim_transition = transition(
    implementation = _transition_impl,
    inputs = ["//command_line_option:extra_toolchains"],
    outputs = [
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
    doc = "The xezim binary, built with Rust 1.100 in opt mode.",
)
