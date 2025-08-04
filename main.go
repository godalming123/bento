package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"
	"github.com/godalming123/bento/utils"
)

type unparsedSourceConfig struct {
	Url                              string
	Compression                      string
	Checksums                        map[string]string
	FilesToMakeExecutable            []string
	RootPath                         string
	Version                          map[string]string
	ArchitectureNames                map[string]string
	Homepage                         string
	Licenses                         []string
	Description                      string
	ProgrammingLanguage              string
	Env                              map[string]map[string]string
	DirectlyDependentSharedLibraries map[string][]string
}

type parsedSourceConfig struct {
	unparsedSourceConfig
	interpolationFunc func(string) (string, error)
	path              string
	parsedUrl         string
	parsedChecksum    [32]byte
	parsedRootPath    string
}

type unparsedLibrary struct {
	Source                           string
	Directory                        string
	DirectlyDependentSharedLibraries []string
}

type parsedLibrary struct {
	absoluteDirectory string
}

type sourceLoadingError struct {
	sourceName string
	message    string
}

func (e *sourceLoadingError) Error() string {
	return "Failed to load source `" + e.sourceName + "`: " + e.message
}

func loadSource(sourcesDirPath string, downloadedSourcesDirPath string, loadedSources map[string]parsedSourceConfig, nameOfSourceToLoad string) (parsedSourceConfig, error) {
	parsedSourceConf, sourceLoaded := loadedSources[nameOfSourceToLoad]
	if sourceLoaded {
		return parsedSourceConf, nil
	}
	contents, err := os.ReadFile(path.Join(sourcesDirPath, nameOfSourceToLoad+".toml"))
	if err != nil {
		return parsedSourceConfig{}, &sourceLoadingError{nameOfSourceToLoad, err.Error()}
	}
	var unparsedSourceConf unparsedSourceConfig
	_, err = toml.Decode(string(contents), &unparsedSourceConf)
	if err != nil {
		return parsedSourceConfig{}, &sourceLoadingError{nameOfSourceToLoad, err.Error()}
	}
	architecture, ok := unparsedSourceConf.ArchitectureNames[runtime.GOARCH]
	if !ok {
		architecture = runtime.GOARCH
	}
	interpolationFunc := func(s string) (string, error) {
		if s == "architecture" {
			return architecture, nil
		} else if trimmedStr, didTrim := utils.TrimPrefix(s, "version."); didTrim {
			version, ok := unparsedSourceConf.Version[trimmedStr]
			if !ok {
				return "", errors.New("No key `" + trimmedStr + "` in version")
			}
			return version, nil
		}
		return "", &sourceLoadingError{nameOfSourceToLoad, "Expected either `architecture`, or `version.` followed by a key in the `version` value. Got " + s}
	}
	parsedUrl, err := utils.InterpolateStringLiteral(unparsedSourceConf.Url, interpolationFunc)
	if err != nil {
		return parsedSourceConfig{}, err
	}
	// Ideally checksum parsing would use https://github.com/BurntSushi/toml/issues/448
	parsedChecksumString, exists := unparsedSourceConf.Checksums[parsedUrl]
	if !exists {
		return parsedSourceConfig{}, &sourceLoadingError{nameOfSourceToLoad, "The checksum for the URL " + parsedUrl + " is not specefied. Bento requires checksums to be specified."}
	}
	if len(parsedChecksumString) != 64 {
		return parsedSourceConfig{}, &sourceLoadingError{nameOfSourceToLoad, "Expected checksum to be 64 charecters, but it is " + fmt.Sprint(len(parsedChecksumString)) + " charecters"}
	}
	parsedChecksumSlice, err := hex.DecodeString(parsedChecksumString)
	if err != nil {
		return parsedSourceConfig{}, &sourceLoadingError{nameOfSourceToLoad, "Failed to decode checksum: " + err.Error()}
	}
	if len(parsedChecksumSlice) != 32 {
		panic("Unexpected internal state: len(parsedChecksumSlice) = " + fmt.Sprint(len(parsedChecksumSlice)))
	}
	var parsedChecksum [32]byte
	copy(parsedChecksum[:], parsedChecksumSlice)
	parsedRootPath, err := utils.InterpolateStringLiteral(unparsedSourceConf.RootPath, interpolationFunc)
	if err != nil {
		return parsedSourceConfig{}, err
	}
	parsedSourceConf = parsedSourceConfig{
		unparsedSourceConfig: unparsedSourceConf,
		interpolationFunc:    interpolationFunc,
		path:                 path.Join(downloadedSourcesDirPath, nameOfSourceToLoad),
		parsedUrl:            parsedUrl,
		parsedChecksum:       parsedChecksum,
		parsedRootPath:       parsedRootPath,
	}
	loadedSources[nameOfSourceToLoad] = parsedSourceConf
	return parsedSourceConf, nil
}

func loadLibrary(
	librariesDirPath string,
	sourcesDirPath string,
	downloadedSourcesDirPath string,
	loadedLibraries map[string]parsedLibrary,
	loadedSources map[string]parsedSourceConfig,
	nameOfLibraryToLoad string,
) error {
	_, libraryLoaded := loadedLibraries[nameOfLibraryToLoad]
	if libraryLoaded {
		return nil
	}
	contents, err := os.ReadFile(path.Join(librariesDirPath, nameOfLibraryToLoad+".toml"))
	if err != nil {
		return errors.New("Failed to load library " + nameOfLibraryToLoad + ": " + err.Error())
	}
	var unparsedLibraryConfig unparsedLibrary
	_, err = toml.Decode(string(contents), &unparsedLibraryConfig)
	if err != nil {
		return errors.New("Failed to load library " + nameOfLibraryToLoad + ": " + err.Error())
	}
	for _, directlyDependentSharedLibrary := range unparsedLibraryConfig.DirectlyDependentSharedLibraries {
		err := loadLibrary(librariesDirPath, sourcesDirPath, downloadedSourcesDirPath, loadedLibraries, loadedSources, directlyDependentSharedLibrary)
		if err != nil {
			return err
		}
	}
	if unparsedLibraryConfig.Source != "system" {
		sourceConf, err := loadSource(sourcesDirPath, downloadedSourcesDirPath, loadedSources, unparsedLibraryConfig.Source)
		if err != nil {
			return errors.New("Failed to load library " + nameOfLibraryToLoad + ": " + err.Error())
		}
		loadedLibraries[nameOfLibraryToLoad] = parsedLibrary{absoluteDirectory: path.Join(sourceConf.path, unparsedLibraryConfig.Directory)}
	}
	return nil
}

func main() {
	index := 1
	subcommand := utils.TakeOneArg(&index, "the subcommand to run (either `help`, `update`, or `exec`)")
	switch subcommand {
	case "help":
		utils.ExpectAllArgsParsed(index)
		// TODO: Improve help message
		println("Bento is a cross-distro package manager that can be used without root. For more information, see https://github.com/godalming123/bento.")
	case "update":
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			utils.Fail("Failed to get cache directory: " + err.Error())
		}
		packageCacheDir := path.Join(cacheDir, "bento")
		utils.ExpectAllArgsParsed(index)
		errs := utils.FetchPackageRepository(packageCacheDir)
		if len(errs) != 0 {
			os.Exit(1)
		}
	case "exec":
		var sourceName, sourceExecutableRelativePath, lastArg string
		lastArgDesc := "Either `--arg` followed by an argument to pass to the " +
			"executable, or the bento directory plus some charecters, `/`, and some " +
			"more charecters (normally this is passed in by `/usr/bin/env`, which " +
			"sends some arguments like [`bento`, `exec`, `SOURCE_NAME`, " +
			"`EXECUTABLE_NAME`, `SCRIPT_PATH`, `ARG1`, ...] when bento is invoked from" +
			"a shebang like `#!/usr/bin/env -S bento exec SOURCE_NAME EXECUTABLE_NAME`)"
		utils.TakeArgs(&index, []utils.Argument{
			{Desc: "The name of the source", Value: &sourceName},
			{Desc: "The path of the executable within the source", Value: &sourceExecutableRelativePath},
			{Desc: lastArgDesc, Value: &lastArg},
		})
		argsToPass := []string{}
		for lastArg == "--arg" {
			var argValue string
			utils.TakeArgs(&index, []utils.Argument{
				{Desc: "The value of the argument to pass to the executable", Value: &argValue},
				{Desc: lastArgDesc, Value: &lastArg},
			})
			argsToPass = append(argsToPass, argValue)
		}
		// For some reason argcomplete (https://github.com/kislyuk/argcomplete/) executes `bento exec SOURCE_NAME EXECUTABLE_NAME -m argcomplete._check_console_script PATH_TO_SCRIPT`, when these 4 conditions are simultaniously met:
		// - Argcomplete is setup in the users shell using the "global completion" strategy
		// - The user has typed the name of a script that is in their path and a space into their shell prompt
		// - The script uses a shebang like `#!/usr/bin/env bento exec SOURCE_NAME EXECUTABLE_NAME`
		// - The user presses tab
		// This causes a problem if bento ignores `argCompleteShenanigans` and the executable EXECUTABLE_NAME runs forever when there is no user input to stdin, because then when the user presses tab to autocomplete options for the script which has a shebang:
		// 1. The users shell executes argcomplete
		// 2. Argcomplete executes bento with the above arguments
		// 3. Bento would execute the executable as normal
		// 4. The users shell would freeze because bento never exits
		// To mitagate this, this condition is necersarry
		if lastArg == "-m" {
			os.Exit(1)
		}
		argsToPass = append(argsToPass, os.Args[index:]...)
		exec(sourceName, sourceExecutableRelativePath, path.Dir(path.Dir(lastArg)), argsToPass)
	default:
		utils.Fail("`" + subcommand + "` is not a valid subcommand. Expected either `help`, `update`, or `exec`")
	}
}

func exec(sourceName string, sourceExecutableRelativePath string, bentoDir string, argsToPass []string) {
	sourcesDir := path.Join(bentoDir, "sources")
	downloadedSourcesDir := path.Join(bentoDir, "downloadedSources")
	librariesDir := path.Join(bentoDir, "lib")

	libraries := map[string]parsedLibrary{}
	sources := map[string]parsedSourceConfig{}
	sourceConf, err := loadSource(sourcesDir, downloadedSourcesDir, sources, sourceName)
	if err != nil {
		utils.Fail(err.Error())
	}
	sourceExecutable := path.Join(sourceConf.path, sourceExecutableRelativePath)

	executableEnvironmentUnparsed := os.Environ()
	executableEnvironment := map[string]string{}
	for _, environmentVariable := range executableEnvironmentUnparsed {
		environmentVariableSplit := strings.SplitN(environmentVariable, "=", 2)
		executableEnvironment[environmentVariableSplit[0]] = environmentVariableSplit[1]
	}

	executableEnvironmentConfig, _ := sourceConf.Env[sourceExecutableRelativePath]
	for envName, envValue := range executableEnvironmentConfig {
		replacedValue, err := utils.InterpolateStringLiteral(envValue, func(interpolation string) (string, error) {
			source, err := loadSource(sourcesDir, downloadedSourcesDir, sources, interpolation)
			if err != nil {
				return "", err
			}
			return source.path, nil
		})
		if err != nil {
			utils.Fail(err.Error())
		}
		executableEnvironment[envName] = replacedValue
	}

	directlyDependentSharedLibraries, _ := sourceConf.DirectlyDependentSharedLibraries[sourceExecutableRelativePath]
	for _, directlyDependentSharedLibrary := range directlyDependentSharedLibraries {
		err := loadLibrary(librariesDir, sourcesDir, downloadedSourcesDir, libraries, sources, directlyDependentSharedLibrary)
		if err != nil {
			utils.Fail(err.Error())
		}
	}

	downloads := make([]utils.DownloadOptions, 0, len(sources))
	for sourceName, sourceConf := range sources {
		_, err := os.Stat(sourceConf.path)
		if os.IsNotExist(err) {
			downloads = append(downloads, utils.DownloadOptions{
				Name:                             sourceName,
				Url:                              sourceConf.parsedUrl,
				Compression:                      sourceConf.Compression,
				Checksum:                         sourceConf.parsedChecksum,
				UseChecksum:                      true,
				FilesToMakeExecutable:            sourceConf.FilesToMakeExecutable,
				RootPath:                         sourceConf.parsedRootPath,
				Destination:                      sourceConf.path,
				DeleteExistingFilesAtDestination: false,
			})
		} else if err != nil {
			utils.Fail("Failed to stat `" + sourceConf.path + "`: " + err.Error())
		}
	}
	if len(downloads) > 0 {
		noun := ""
		if len(downloads) == 1 {
			noun = "source"
		} else {
			noun = strconv.FormatInt(int64(len(downloads)), 10) + " sources"
		}
		println("Download the following " + noun + " to run the binary " + sourceExecutableRelativePath + " from the source " + sourceName + "?")
		for _, download := range downloads {
			println(" - " + download.Name + " from " + download.Url)
		}
		if !utils.GetBoolDefaultYes() {
			return
		}
		errs := utils.DownloadConcurrently(downloads)
		if len(errs) > 0 {
			os.Exit(1)
		}
	}

	// Use a hash map to de-duplicate libraries with the same path
	librariesPathsMap := map[string]struct{}{}
	for _, library := range libraries {
		librariesPathsMap[library.absoluteDirectory] = struct{}{}
	}
	librariesPathsList := utils.Collect(maps.Keys(librariesPathsMap))
	executableEnvironment["LD_LIBRARY_PATH"] = strings.Join(librariesPathsList, ":")

	executableEnv := make([]string, 0, len(executableEnvironment))
	for key, value := range executableEnvironment {
		executableEnv = append(executableEnv, key+"="+value)
	}
	err = syscall.Exec(sourceExecutable, append([]string{sourceExecutable}, argsToPass...), executableEnv)
	if err != nil {
		utils.Fail("Failed to execute binary `" + sourceExecutable + "`: " + err.Error())
	}
}
