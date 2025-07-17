# Exec bin

> [!WARNING]
> Exec bin is pre-alpha.

Execute other binaries with a focus on:

- Having lots of packages (although currently there are less than a dozen)
- Just working on every linux distro and macOS (although I cannot test on mac)
- Being fast at installing and running packages
- Being small (installed packages do not take too much space)
- Being minimal (exec bin itself is a single, statically-linked binary)
- Not polluting the host system (all packages install to the same cache directory, which at any point can be completely deleted)
- Being able to have multiple version of the same package installed at the same time

## Todo

- Support installing different versions of the same package at the same time
- Support fetching a list of versions of a package from github
- Implement cryptographic verification of sources
- Get the user to agree to the license of the sources before they are fetched and extracted
- Add a way to add entries to the `LD_LIBRARY_PATH` environment variable, so that executables that use dynamically linked libaries (like the one for ghostty) can use libraries from exec-bin instead of system libraries
  - There could be an automation for [binary-repository](https://github.com/godalming123/binary-repository/) that uses `ldd` to check that all of the dynamically linked libraries that packaged executables require are packaged

## Installation

### From binary

TODO: Add a pre-built binary

### Building from source

```sh
git clone --depth 1 https://github.com/godalming123/exec-bin.git
cd exec-bin
go install
```

# Stargazers over time

[![Stargazers over time](https://starchart.cc/godalming123/exec-bin.svg)](https://starchart.cc/godalming123/exec-bin)
