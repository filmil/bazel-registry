# SPDX-License-Identifier: Apache-2.0
"""Module extension that sets up the hugo binary repository.

The `hugo_repository` repository rule predates bzlmod; this extension makes it
usable from MODULE.bazel:

    hugo = use_extension("@rules_hugo//hugo:extensions.bzl", "hugo")
    hugo.download(version = "0.91.2", extended = True, sha256 = "...")
    use_repo(hugo, "hugo")

The generated repository is named `hugo` by default, which is what the
`hugo_site` rule's `hugo` attribute points at.
"""

load("//hugo:internal/hugo_repository.bzl", "hugo_repository")

_download = tag_class(
    doc = "Declares the hugo release binary to download.",
    attrs = {
        "name": attr.string(
            default = "hugo",
            doc = "Name of the generated repository.",
        ),
        "version": attr.string(
            mandatory = True,
            doc = "The hugo version to download, e.g. \"0.91.2\".",
        ),
        "sha256": attr.string(
            doc = "sha256 of the release archive for this platform.",
        ),
        "extended": attr.bool(
            default = False,
            doc = "Download the extended hugo build.",
        ),
        "os_arch": attr.string(
            doc = "Override the os/arch archive suffix; autodetected when empty.",
        ),
    },
)

def _hugo_impl(mctx):
    seen = {}
    for mod in mctx.modules:
        for tag in mod.tags.download:
            if tag.name in seen:
                # First declaration wins; root modules are iterated first.
                continue
            seen[tag.name] = True
            hugo_repository(
                name = tag.name,
                version = tag.version,
                sha256 = tag.sha256,
                extended = tag.extended,
                os_arch = tag.os_arch,
            )

hugo = module_extension(
    implementation = _hugo_impl,
    tag_classes = {"download": _download},
)
