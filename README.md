# Bzl Bazel Registry

A personal Bazel registry.

This registry hosts Bazel modules that are either not suitable for the Bazel
Central Registry (BCR) or are under development. As Bazel moves towards Bzlmod
and phases out `WORKSPACE` files, this registry provides a home for bespoke
modules.

## Usage

Add the registry to your `.bazelrc` file. Note that adding a custom registry
removes the BCR default entry, so you must add it back to retain access to
the items published in the BCR.

```
build --registry=https://raw.githubusercontent.com/filmil/bazel-registry/main \
      --registry=https://bcr.bazel.build
```

## License

This project is licensed under the terms of the [LICENSE](LICENSE) file.
