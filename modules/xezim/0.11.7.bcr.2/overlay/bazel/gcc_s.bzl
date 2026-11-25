"""A directory that holds a static archive as libgcc_s.a."""

def _gcc_s_shim_impl(ctx):
    out = ctx.actions.declare_directory(ctx.label.name)
    ctx.actions.run_shell(
        inputs = [ctx.file.archive],
        outputs = [out],
        command = 'cp "$1" "$2/libgcc_s.a"',
        arguments = [ctx.file.archive.path, out.path],
        mnemonic = "GccSShim",
        progress_message = "Placing %{label}",
    )
    return [DefaultInfo(files = depset([out]))]

gcc_s_shim = rule(
    implementation = _gcc_s_shim_impl,
    doc = "Copies a static archive into a directory as libgcc_s.a, for -L.",
    attrs = {
        "archive": attr.label(allow_single_file = [".a"], mandatory = True),
    },
)
