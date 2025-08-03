# bazel-registry

A Bazel registry for my work.

Bazel 9.0.0 will turn off support for `WORKSPACE`. This should happen later in 2025,
so we need to migrate our own work away from `WORKSPACE` files.

Ideally, all your bazel modules can go to bazel central registy. However, some
modules end up not really being acceptable to BCR, since they aren't general
enough, or aren't important enough to others, or (as in my case) use approaches
that don't work well in the BCR ecosystem.

I spun up this registry to have a home for my modules. Feel free to use it
under the terms of the enclosed [LICENSE][lic].

[lic]: ./LICENSE
