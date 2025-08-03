# Bzl Bazel Registry

A personal Bazel registry.

This registry hosts Bazel modules that are either not suitable for the Bazel Central Registry (BCR) or are under development. As Bazel moves towards Bzlmod and phases out `WORKSPACE` files, this registry provides a home for bespoke modules.

## Usage

To use this registry, add the following to your `MODULE.bazel` file:

```bzl
bazel_dep(name = "bazel_ebook", version = "0.0.5")
bazel_dep(name = "bazel_rules_bid", version = "0.2.5")
bazel_dep(name = "bazel_rules_bt", version = "0.0.3")
```

Then, add the registry to your `.bazelrc` file:

```
build --registry=https://bcr.bazel.build --registry=https://raw.githubusercontent.com/filmil/bazel-registry/main
```

## Modules

This registry contains the following modules:

*   `bazel_ebook`: A Bazel module for building ebooks.
*   `bazel_rules_bid`: A Bazel module for BID.
*   `bazel_rules_bt`: A Bazel module for BT.

## Contributing

Contributions are welcome! Please see the [contributing guidelines](GEMINI.md) for more information.

## License

This project is licensed under the terms of the [LICENSE](LICENSE) file.