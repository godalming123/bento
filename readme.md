# Exec bin

> [!WARNING]
> Exec bin is pre-alpha.

Execute other binaries with a focus on:

- Having lots of packages (although currently there are only 2)
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
- Fetch the package repository if it does not exist
- Support zip compression
- Get the user to agree to the license of the sources before they are fetched and extracted
