package main

import (
	"errors"
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
	Checksums                        map[string][32]byte
	FilesToMakeExecutable            []string
	RootPath                         string
	Version                          map[string]string
	ArchitectureNames                map[string]string
	Homepage                         string
	Env                              map[string]map[string]string
	DirectlyDependentSharedLibraries map[string][]string
}

type parsedSourceConfig struct {
	unparsedSourceConfig
	interpolationFunc func(string) (string, error)
	path              string
	parsedUrl         string
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
		return "", errors.New("Expected either `architecture`, or `version.` followed by a key in the `version` value. Got " + s)
	}
	parsedUrl, err := utils.InterpolateStringLiteral(unparsedSourceConf.Url, interpolationFunc)
	if err != nil {
		return parsedSourceConfig{}, err
	}
	parsedRootPath, err := utils.InterpolateStringLiteral(unparsedSourceConf.RootPath, interpolationFunc)
	if err != nil {
		return parsedSourceConfig{}, err
	}
	parsedSourceConf = parsedSourceConfig{
		unparsedSourceConfig: unparsedSourceConf,
		interpolationFunc:    interpolationFunc,
		path:                 path.Join(downloadedSourcesDirPath, nameOfSourceToLoad),
		parsedUrl:            parsedUrl,
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

type argument struct {
	description string
	value       *string
}

func main() {
	index := 1
	subcommand := utils.TakeOneArg(&index, "the subcommand to run (either `help`, `update`, or `exec`)")
	if subcommand == "help" {
		utils.ExpectAllArgsParsed(index)
		utils.Fail("TODO: Add help message")
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		utils.Fail("Failed to get cache directory: " + err.Error())
	}
	packageCacheDir := path.Join(cacheDir, "bento")
	if subcommand == "update" {
		utils.ExpectAllArgsParsed(index)
		errs := utils.FetchPackageRepository(packageCacheDir)
		if len(errs) != 0 {
			os.Exit(1)
		}
		return
	}
	if subcommand != "exec" {
		utils.Fail("`" + subcommand + "` is not a valid subcommand. Subcommands are `help`, `update`, and `exec`")
	}
	var sourceName, sourceExecutableRelativePath string
	utils.TakeArgs(&index, []utils.Argument{
		{Desc: "The name of the source", Value: &sourceName},
		{Desc: "The path of the executable within the source", Value: &sourceExecutableRelativePath},
		{Desc: "Any value (this is ignored by bento) (this is necersarry because when " +
			"bento is invoked from a shebang like `#!/usr/bin/env -S bento exec " +
			"SOURCE_NAME EXECUTABLE_NAME`, bento will receive the arguments [`bento`, " +
			"`exec`, `SOURCE_NAME`, `EXECUTABLE_NAME`, `ARG0`, `ARG1`, ...], and in " +
			"this case, bento must ignore `ARG0`, and instead pass a different `ARG0` " +
			"to the executable."},
	})

	sourcesDir := path.Join(packageCacheDir, "sources")
	downloadedSourcesDir := path.Join(packageCacheDir, "downloadedSources")
	librariesDir := path.Join(packageCacheDir, "lib")

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
				UseChecksum:                      false, // TODO: Waiting for https://github.com/BurntSushi/toml/issues/448 to be implemented to add cryptographic verification
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
	err = syscall.Exec(sourceExecutable, append([]string{sourceExecutable}, os.Args[index:]...), executableEnv)
	if err != nil {
		utils.Fail("Failed to execute binary `" + sourceExecutable + "`: " + err.Error())
	}
}
