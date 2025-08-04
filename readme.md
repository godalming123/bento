# Bento

A package manager with a focus on:

- Having lots of packages (although currently there are less than a hundred)
- Just working on every linux distro
- Not necesitating root permisions to be installed, and to install packages
- Being fast at installing and running packages
- Being small (installed packages do not take too much space)
- Being minimal (bento itself is a single, statically-linked binary)
- Not polluting the host system (all packages install to the same cache directory)
- Being able to have multiple version of the same package installed at the same time (TODO)

## Todo

- Support installing different versions of the same package at the same time
- Support using different versions of a package by setting environment variables like `BENTO_helix_VERSION`
- Support fetching a list of versions of a package from github
- Implement cryptographic verification of sources
- Update the list of sources that is displayed to get user confirmation to download sources:
  - Group the sources by their license
  - Show extra information about the sources:
    - The description of the source
    - Possibly which programming language the source is written in
    - Possibly the homepage of the source
  - Properly wrap text that overflows
- Add automated testing for [binary-repository](https://github.com/godalming123/binary-repository/)
- Consider adding an automation for [binary-repository](https://github.com/godalming123/binary-repository/) that uses `ldd` to check that all of the dynamically linked libraries that packaged executables require are packaged
- Possibly add support for other unix based operating systems, including freeBSD and macOS

## Current limitations

If any of these limitations mean that I cannot package something, then I will remove the limitation:

- Binary names must be exclusive
- Library names must be exclusive
- There are no post install scripts (I don't like post install scripts becuase they can sometimes be the longest part of a software installation, and there is no way to tell how long they will take)

## Installation

### 1. Install `bento`

```sh
sudo curl --location https://github.com/godalming123/bento/releases/latest/download/linux-amd64 -o /usr/bin/bento
sudo chmod +x /usr/bin/bento
```

<details><summary>Building from source</summary>

```sh
git clone --depth 1 https://github.com/godalming123/bento.git
cd bento
go install
# `go install` puts the binary in `~/go/bin`, so make sure that directory is in your $PATH
```

</details>

### 2. Download the package repository

```sh
bento update
```

### 3. Add the package repository to your `PATH`

`~/.bashrc`

```diff
+export PATH="$HOME/.cache/bento/bin:$PATH"
```

## Running bento packages as root

TODO: Add documentation for how to use priviledge managers other than `sudo`.

```sh
sudo sh -c "PATH=$PATH COMMAND_NAME COMMAND_ARGS"
```

For example:

```sh
sudo sh -c "PATH=$PATH hx FILE_OWNED_BY_ROOT"
```

If bento is installed to `/usr/bin`, then you can also use the slightly shorter:

```sh
sudo $(which COMMAND_NAME) COMMAND_ARGS
```

## Stargazers over time

[![Stargazers over time](https://starchart.cc/godalming123/bento.svg)](https://starchart.cc/godalming123/bento)
