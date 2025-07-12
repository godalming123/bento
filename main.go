package main

import (
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"
)

type sourceConfig struct {
	Url                   string
	Compression           string
	Checksums             map[string]string
	FilesToMakeExecutable []string
	RootPath              string
	Version               string
}

func main() {
	if len(os.Args) < 2 {
		fail("Expected argument for name of binary to run")
	}

	binaryName := os.Args[1]

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		fail("Failed to get cache directory: " + err.Error())
	}

	packageCacheDir := path.Join(cacheDir, "exec-bin")
	binariesDir := path.Join(packageCacheDir, "bin")
	binarySymlinkPath := path.Join(binariesDir, binaryName)
	binaryRelativePathFromLink, err := os.Readlink(binarySymlinkPath)
	if os.IsNotExist(err) {
		fail("TODO: If the package repository (" + packageCacheDir + ") does not exist, then fetch it and retry\n" +
			"TODO: If there is still an error, tell the user that `binaryName` does not exist and that updating the package repository might fix this")
	} else if err != nil {
		fail("Failed to read the link `" + binarySymlinkPath + "`: " + err.Error())
	}
	binaryRelativePathFromSourcesDirectory, ok := trimPrefix(binaryRelativePathFromLink, "../sources/")
	if !ok {
		fail("Expected the executable that the link at `" + binarySymlinkPath + "` points to to start with `../sources/`, but it is `" + binaryRelativePathFromLink + "`")
	}
	sourceName := strings.Split(binaryRelativePathFromSourcesDirectory, "/")[0]
	sourceDir := path.Join(packageCacheDir, "sources", sourceName)

	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		sourcesFilePath := path.Join(packageCacheDir, "sources.toml")
		contents, err := os.ReadFile(sourcesFilePath)
		if err != nil {
			fail("Failed to read sources file `" + sourcesFilePath + "`: " + err.Error())
		}
		architecture := runtime.GOARCH
		if architecture == "amd64" {
			architecture = "x86_64"
		}

		var sourceConfigs map[string]sourceConfig
		_, err = toml.Decode(string(contents), &sourceConfigs)
		if err != nil {
			fail("Failed to read package config `" + sourcesFilePath + "`: " + err.Error())
		}
		sourceConfig, ok := sourceConfigs[sourceName]
		if !ok {
			fail("There is no config for a source called `" + sourceName + "` in `" + sourcesFilePath + "`")
		}
		replacer := strings.NewReplacer("${version}", sourceConfig.Version, "${architecture}", architecture, "${os}", runtime.GOOS)
		sourceUrl := replacer.Replace(sourceConfig.Url)
		// TODO: Show progress
		println("Fetching " + sourceUrl)
		response, err := http.Get(sourceUrl)
		if err != nil {
			fail("Failed to fetch `" + sourceUrl + "`: " + err.Error())
		}
		defer response.Body.Close()
		println("Extracting")
		err = extract(response.Body, sourceConfig.Compression, sourceDir, replacer.Replace(sourceConfig.RootPath))
		if err != nil {
			fail("Failed to extract: ", err.Error())
		}

		for _, fileName := range sourceConfig.FilesToMakeExecutable {
			absoluteFileName := path.Join(sourceDir, fileName)
			fileInfo, err := os.Stat(absoluteFileName)
			if err != nil {
				fail("Failed to make the file `" + fileName + "` executable: " + err.Error())
			}
			err = os.Chmod(absoluteFileName, fileInfo.Mode()|0111)
			if err != nil {
				fail("Failed to make the file `" + fileName + "` executable: " + err.Error())
			}
		}
	}

	binaryAbsolutePath := path.Join(binariesDir, binaryRelativePathFromLink)
	err = syscall.Exec(binaryAbsolutePath, append([]string{binaryAbsolutePath}, os.Args[2:]...), os.Environ())
	if err != nil {
		fail("Failed to execute binary `" + binaryAbsolutePath + "`: " + err.Error())
	}
}
