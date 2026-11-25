"""The toolchains xezim builds with.

- Rust 1.100. rules_rust takes the Rust version from the root module only,
  and its default (1.99.1 in rules_rust 0.75.0) is older than the 1.100 that
  xezim needs.
- clang from LLVM, with a pinned Debian bullseye sysroot, for the C code (the shims,
  zstd, mimalloc, libffi) and for the link. Neither comes from the host.
  toolchains_llvm's own extension is for the root module only.

Neither toolchain is registered: only //:xezim uses them, through the
transition in tool.bzl.
"""

# rules_rust 0.75.0 has no public load for this. A newer rules_rust picked by
# version resolution may move it; this module then needs an update.
load("@rules_rust//rust/private:repositories.bzl", "rust_repository_set")
load("@toolchains_llvm//toolchain:rules.bzl", "llvm_toolchain")
load("@toolchains_llvm//toolchain:sysroot.bzl", "sysroot")

RUST_VERSION = "1.100.0"

LLVM_VERSION = "23.1.5"

# Pinned here, not taken from toolchains_llvm's own table, so that a newer
# toolchains_llvm cannot change what is downloaded.
_LLVM_ARCHIVE = "LLVM-23.1.5-Linux-X64"
_LLVM_SHA256 = "f72edb0b36580ece7f52e23858f00c18fad4e87395ea7ad05d7e55c39f8fc78a"

# From https://static.rust-lang.org/dist/<archive>.sha256
_SHA256S = {
    "cargo-1.100.0-x86_64-unknown-linux-gnu.tar.xz": "7e29ce07d7c6e92d1826870575ba27d57fff6cb3e754aa57d99099cb88fab35d",
    "clippy-1.100.0-x86_64-unknown-linux-gnu.tar.xz": "0d18df3bff6c852ef8243727fed075ecb33dbf93b8ab8c138c2f45e3f270102a",
    "llvm-tools-1.100.0-x86_64-unknown-linux-gnu.tar.xz": "2c84cada1aef73339b1a2d101a35cd78d71072c8f6a3921bf1abb852c3f42ad3",
    "rust-std-1.100.0-x86_64-unknown-linux-gnu.tar.xz": "4591b6486594ded52ecd6135db95bf9355bbd3ca7a380888b141528bbcf9a5d4",
    "rustc-1.100.0-x86_64-unknown-linux-gnu.tar.xz": "f1d0b1dec81dab6f523cd098507763487b0d4bd7fd2c87582f154128719be03b",
}

def _xezim_toolchains_impl(module_ctx):
    # Chromium's Debian bullseye sysroot (glibc 2.31), as listed in Chromium's
    # build/linux/sysroot_scripts/sysroots.json. libffi 3.8 needs glibc 2.27
    # or newer for memfd_create. Chromium keeps its sysroots by their hash;
    # there is no second mirror.
    sysroot(
        name = "xezim_sysroot",
        sha256 = "f851add76bdae472fb6c94b4f920bc4a132d85ac607951391297f636af7e934e",
        urls = ["https://commondatastorage.googleapis.com/chrome-linux-sysroot/f851add76bdae472fb6c94b4f920bc4a132d85ac607951391297f636af7e934e"],
    )
    llvm_toolchain(
        name = "xezim_llvm",
        exec_arch = "amd64",
        exec_os = "linux",
        sha256 = {"linux-x86_64": _LLVM_SHA256},
        strip_prefix = {"linux-x86_64": _LLVM_ARCHIVE},
        urls = {"linux-x86_64": [
            "https://github.com/llvm/llvm-project/releases/download/llvmorg-{}/{}.tar.xz".format(LLVM_VERSION, _LLVM_ARCHIVE),
        ]},
        # The Chromium sysroots use Debian's header and library layout.
        cxx_include_layout = {"linux-x86_64": "debian"},
        llvm_version = LLVM_VERSION,
        # toolchains_llvm reads this label in its own repository, so it has to
        # be canonical. MODULE.bazel's use_repo makes @xezim_sysroot resolvable.
        sysroot = {"linux-x86_64": str(Label("@xezim_sysroot//sysroot"))},
    )
    rust_repository_set(
        name = "xezim_rust",
        versions = [RUST_VERSION],
        exec_triple = "x86_64-unknown-linux-gnu",
        edition = "2024",
        sha256s = _SHA256S,
    )
    return module_ctx.extension_metadata(reproducible = True)

xezim_toolchains = module_extension(
    implementation = _xezim_toolchains_impl,
    doc = "Makes the Rust 1.100 and clang toolchains that //:xezim builds with.",
)
