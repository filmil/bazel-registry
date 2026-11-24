"""A Rust 1.100 toolchain for building xezim.

rules_rust takes the Rust version from the root module only, and its default
(1.99.1 in rules_rust 0.75.0) is older than the 1.100 that xezim needs. This
extension makes a toolchain of its own. It is not registered: only //:xezim
uses it, through the transition in tool.bzl.
"""

# rules_rust 0.75.0 has no public load for this. A newer rules_rust picked by
# version resolution may move it; this module then needs an update.
load("@rules_rust//rust/private:repositories.bzl", "rust_repository_set")

RUST_VERSION = "1.100.0"

# From https://static.rust-lang.org/dist/<archive>.sha256
_SHA256S = {
    "cargo-1.100.0-x86_64-unknown-linux-gnu.tar.xz": "7e29ce07d7c6e92d1826870575ba27d57fff6cb3e754aa57d99099cb88fab35d",
    "clippy-1.100.0-x86_64-unknown-linux-gnu.tar.xz": "0d18df3bff6c852ef8243727fed075ecb33dbf93b8ab8c138c2f45e3f270102a",
    "llvm-tools-1.100.0-x86_64-unknown-linux-gnu.tar.xz": "2c84cada1aef73339b1a2d101a35cd78d71072c8f6a3921bf1abb852c3f42ad3",
    "rust-std-1.100.0-x86_64-unknown-linux-gnu.tar.xz": "4591b6486594ded52ecd6135db95bf9355bbd3ca7a380888b141528bbcf9a5d4",
    "rustc-1.100.0-x86_64-unknown-linux-gnu.tar.xz": "f1d0b1dec81dab6f523cd098507763487b0d4bd7fd2c87582f154128719be03b",
}

def _xezim_rust_impl(module_ctx):
    rust_repository_set(
        name = "xezim_rust",
        versions = [RUST_VERSION],
        exec_triple = "x86_64-unknown-linux-gnu",
        edition = "2024",
        sha256s = _SHA256S,
    )
    return module_ctx.extension_metadata(reproducible = True)

xezim_rust = module_extension(
    implementation = _xezim_rust_impl,
    doc = "Makes the Rust 1.100 toolchain that //:xezim builds with.",
)
