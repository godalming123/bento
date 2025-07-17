package main

import (
	// "crypto/sha256"
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
	Checksums             map[string][32]byte
	FilesToMakeExecutable []string
	RootPath              string
	Version               map[string]string
	ArchitectureNames     map[string]string
	Homepage              string
}

func main() {
	if len(os.Args) < 2 {
		fail("Expected argument for name of binary or xb command to run")
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		fail("Failed to get cache directory: " + err.Error())
	}
	packageCacheDir := path.Join(cacheDir, "exec-bin")
	if os.Args[1] == "update" {
		err := fetchPackageRepository(packageCacheDir)
		if err != nil {
			fail(err.Error())
		}
		return
	}

	binaryName := os.Args[1]
	binariesDir := path.Join(packageCacheDir, "bin")
	binarySymlinkPath := path.Join(binariesDir, binaryName)
	binaryRelativePathFromLink, err := os.Readlink(binarySymlinkPath)
	if os.IsNotExist(err) {
		_, err := os.Stat(packageCacheDir)
		if os.IsNotExist(err) {
			err = fetchPackageRepository(packageCacheDir)
			if err != nil {
				fail(err.Error())
			}
			binaryRelativePathFromLink, err = os.Readlink(binarySymlinkPath)
			if os.IsNotExist(err) {
				fail("There is no binary called `" + binaryName + "` in `" + binariesDir + "`. Updating the package repository with `xb update` might fix this.")
			} else if err != nil {
				fail(err.Error())
			}
		} else if err != nil {
			fail(err.Error())
		} else {
			fail("There is no binary called `" + binaryName + "` in `" + binariesDir + "`. Updating the package repository with `xb update` might fix this.")
		}
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
		println("Reading sources config from " + sourcesFilePath)
		contents, err := os.ReadFile(sourcesFilePath)
		if err != nil {
			fail("Failed to read sources file `" + sourcesFilePath + "`: " + err.Error())
		}
		var sourceConfigs map[string]sourceConfig
		_, err = toml.Decode(string(contents), &sourceConfigs)
		if err != nil {
			fail("Failed to read sources config `" + sourcesFilePath + "`: " + err.Error())
		}
		sourceConfig, ok := sourceConfigs[sourceName]
		if !ok {
			fail("There is no config for a source called `" + sourceName + "` in `" + sourcesFilePath + "`")
		}
		architecture, ok := sourceConfig.ArchitectureNames[runtime.GOARCH]
		if !ok {
			architecture = runtime.GOARCH
		}
		replacements := []string{"${architecture}", architecture, "${os}", runtime.GOOS}
		for versionKey, version := range sourceConfig.Version {
			replacements = append(replacements, "${version."+versionKey+"}", version)
		}
		replacer := strings.NewReplacer(replacements...)
		sourceUrl := replacer.Replace(sourceConfig.Url)

		// TODO: Show progress
		println("Fetching from " + sourceUrl)
		response, err := fetch(sourceUrl)
		if err != nil {
			fail(err.Error())
		}

		// TODO: Waiting for https://github.com/BurntSushi/toml/issues/448 to be implemented to add cryptographic verification
		// println("Cryptographically verifying source using sha256 hash")
		// dataChecksum := sha256.Sum256(response)
		// checkSumKey := replacer.Replace("${architecture}-${os}")
		// for _, version := range sourceConfig.Version {
		// 	checkSumKey += "-" + version
		// }
		// expectedDataChecksum, ok := sourceConfig.Checksums[checkSumKey]
		// if !ok {
		//	 fail("The key `\"" + sourceName + "\".checksums.\"" + checkSumKey + "\"` does not exist in `" + sourcesFilePath + "`. Please specify this key to be " + string(dataChecksum[:]) + " so that exec-bin can crytographically verify the source called `" + sourceName + "`.")
		// }
		// if dataChecksum != expectedDataChecksum {
		// 	fail("Expected sha256 checksum of source to be " + string(expectedDataChecksum[:]) + ", but got " + string(dataChecksum[:]))
		// }

		println("Extracting into " + sourceDir)
		err = extract(response, sourceConfig.Compression, sourceDir, replacer.Replace(sourceConfig.RootPath))
		if err != nil {
			fail("Failed to extract: ", err.Error())
		}

		println("Making every file listed in the `" + sourceName + ".filesToMakeExecutable` key executable")
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
