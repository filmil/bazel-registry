# Bzl Bazel Registry

Home page at: https://hdlfactory.com/bazel-registry

![Build](https://github.com/filmil/bazel-registry/actions/workflows/build.yml/badge.svg)
![Release](https://github.com/filmil/bazel-registry/actions/workflows/release.yml/badge.svg)

A personal Bazel registry.

This registry hosts Bazel modules that are either not suitable for the Bazel
Central Registry (BCR) or are under development. As Bazel moves towards Bzlmod
and phases out `WORKSPACE` files, this registry provides a home for bespoke
modules.

None of the modules published here are published with any sort of guarantee
whatsoever. Use at your own risk.

See the overall description at the [project page][pp].

[pp]: https://hdlfactory.com/post/2025/09/29/getting-ready-for-the-brave-new-bazel-modules-world/

## Usage

Add the registry to your `.bazelrc` file.

```
common --registry=https://bcr.bazel.build
common --registry=https://raw.githubusercontent.com/filmil/bazel-registry/main
```

### Notes

* Adding a custom registry removes the BCR default entry, so you must add it
  back to retain access to the items published in the BCR.
* The first registry which has the required module is consulted. You may want
  to adjust the ordering of registries.
* Publishing to BCR can take a *long* time. I sometimes publish modules in advance
  to my registry, where the publication bar is lower. Use at your own risk.

## License

This project is licensed under the terms of the [LICENSE](LICENSE) file.
