package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"runtime"
	"slices"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"
	"github.com/godalming123/bento/utils"
)

type unparsedSourceConfig struct {
	UrlInMirror                     string
	Mirrors                         []string
	Compression                     string
	Checksums                       map[string]string
	FilesToMakeExecutable           []string
	RootPath                        string
	Version                         map[string]string
	ArchitectureNames               map[string]string
	Homepage                        string
	Licenses                        []string
	Description                     string
	ProgrammingLanguage             string
	Env                             map[string]map[string]string
	DirectSharedLibraryDependencies map[string][]string
	ExecutableDependencies          [][2]string
	InstallationWarnings            []string
	KnownIssues                     []string
}

type parsedSourceConfig struct {
	compression                     string
	filesToMakeExecutable           []string
	env                             map[string]map[string]string
	directSharedLibraryDependencies map[string][]string
	executableDependencies          [][2]string
	installationWarnings            []string

	licenseDescription string
	interpolationFunc  func(string) (string, error)
	path               string
	parsedUrls         []string
	parsedChecksum     [32]byte
	parsedRootPath     string
}

type unparsedLibrary struct {
	Source                          string
	Directory                       string
	DirectSharedLibraryDependencies []string
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

	licenseDescription := ""
	switch len(unparsedSourceConf.Licenses) {
	case 0:
		licenseDescription = "with an unknown license"
	case 1:
		licenseDescription = "licensed under " + unparsedSourceConf.Licenses[0]
	default:
		licenseDescription = "licensed under "
		slices.Sort(unparsedSourceConf.Licenses)
		for _, license := range unparsedSourceConf.Licenses[0 : len(unparsedSourceConf.Licenses)-2] {
			licenseDescription += license + ", "
		}
		licenseDescription += "and " + unparsedSourceConf.Licenses[len(unparsedSourceConf.Licenses)-1]
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

	urlInMirror, err := utils.InterpolateStringLiteral(unparsedSourceConf.UrlInMirror, interpolationFunc)
	if err != nil {
		return parsedSourceConfig{}, err
	}

	// Ideally checksum parsing would use https://github.com/BurntSushi/toml/issues/448
	checksumString, exists := unparsedSourceConf.Checksums[urlInMirror]
	if !exists {
		return parsedSourceConfig{}, &sourceLoadingError{nameOfSourceToLoad, "The checksum for " + urlInMirror + " is not specified. Bento requires checksums to be specified."}
	}
	if len(checksumString) != 64 {
		return parsedSourceConfig{}, &sourceLoadingError{nameOfSourceToLoad, "Expected checksum to be 64 characters, but it is " + fmt.Sprint(len(checksumString)) + " characters"}
	}
	checksumSlice, err := hex.DecodeString(checksumString)
	if err != nil {
		return parsedSourceConfig{}, &sourceLoadingError{nameOfSourceToLoad, "Failed to decode checksum: " + err.Error()}
	}
	if len(checksumSlice) != 32 {
		panic("Unexpected internal state: len(parsedChecksumSlice) = " + fmt.Sprint(len(checksumSlice)))
	}
	var checksum [32]byte
	copy(checksum[:], checksumSlice)

	rootPath, err := utils.InterpolateStringLiteral(unparsedSourceConf.RootPath, interpolationFunc)
	if err != nil {
		return parsedSourceConfig{}, err
	}

	urls := make([]string, len(unparsedSourceConf.Mirrors))
	for i, mirror := range unparsedSourceConf.Mirrors {
		urls[i] = mirror + "/" + urlInMirror
	}

	parsedSourceConf = parsedSourceConfig{
		compression:                     unparsedSourceConf.Compression,
		filesToMakeExecutable:           unparsedSourceConf.FilesToMakeExecutable,
		env:                             unparsedSourceConf.Env,
		directSharedLibraryDependencies: unparsedSourceConf.DirectSharedLibraryDependencies,
		executableDependencies:          unparsedSourceConf.ExecutableDependencies,
		installationWarnings:            unparsedSourceConf.InstallationWarnings,
		licenseDescription:              licenseDescription,
		interpolationFunc:               interpolationFunc,
		path:                            path.Join(downloadedSourcesDirPath, nameOfSourceToLoad),
		parsedUrls:                      utils.ShuffleSlice(urls),
		parsedChecksum:                  checksum,
		parsedRootPath:                  rootPath,
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
	for _, directSharedLibraryDependency := range unparsedLibraryConfig.DirectSharedLibraryDependencies {
		err := loadLibrary(librariesDirPath, sourcesDirPath, downloadedSourcesDirPath, loadedLibraries, loadedSources, directSharedLibraryDependency)
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

const maxParrellelDownloads = 10

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
		errs := utils.FetchPackageRepository(packageCacheDir, maxParrellelDownloads)
		if len(errs) != 0 {
			os.Exit(1)
		}
	case "exec":
		var sourceName, sourceExecutableRelativePath, lastArg string
		lastArgDesc := "Either `--arg` followed by an argument to pass to the " +
			"executable, or the bento directory plus some characters, `/`, and some " +
			"more characters (normally this is passed in by `/usr/bin/env`, which " +
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
		// For some reason argcomplete (https://github.com/kislyuk/argcomplete/) executes `bento exec SOURCE_NAME EXECUTABLE_NAME -m argcomplete._check_console_script PATH_TO_SCRIPT`, when these 4 conditions are simultaneously met:
		// - Argcomplete is setup in the users shell using the "global completion" strategy
		// - The user has typed the name of a script that is in their path and a space into their shell prompt
		// - The script uses a shebang like `#!/usr/bin/env bento exec SOURCE_NAME EXECUTABLE_NAME`
		// - The user presses tab
		// This causes a problem if bento ignores `lastArg` and the executable EXECUTABLE_NAME runs forever when there is no user input to stdin, because then when the user presses tab to autocomplete options for the script which has a shebang:
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

func loadExecutable(
	sourcesDir string,
	downloadedSourcesDir string,
	loadedSources map[string]parsedSourceConfig,

	librariesDir string,
	loadedLibraries map[string]parsedLibrary,

	sourceName string,
	sourceExecutableRelativePath string,
	loadedExecutables map[string]string,
	executableEnvironment map[string]string,
) (string, error) {
	if executable, ok := loadedExecutables[sourceName+" "+sourceExecutableRelativePath]; ok {
		return executable, nil
	}

	sourceConf, err := loadSource(sourcesDir, downloadedSourcesDir, loadedSources, sourceName)
	if err != nil {
		return "", err
	}
	sourceExecutable := path.Join(sourceConf.path, sourceExecutableRelativePath)

	for _, executable := range sourceConf.executableDependencies {
		_, err := loadExecutable(
			sourcesDir,
			downloadedSourcesDir,
			loadedSources,
			librariesDir,
			loadedLibraries,
			executable[0],
			executable[1],
			loadedExecutables,
			executableEnvironment,
		)
		if err != nil {
			return "", err
		}
	}

	executableEnvironmentConfig, _ := sourceConf.env[sourceExecutableRelativePath]
	for envName, envValue := range executableEnvironmentConfig {
		replacedValue, err := utils.InterpolateStringLiteral(envValue, func(interpolation string) (string, error) {
			source, err := loadSource(sourcesDir, downloadedSourcesDir, loadedSources, interpolation)
			if err != nil {
				return "", err
			}
			return source.path, nil
		})
		if err != nil {
			return "", err
		}
		executableEnvironment[envName] = replacedValue
	}

	directSharedLibraryDependencies, _ := sourceConf.directSharedLibraryDependencies[sourceExecutableRelativePath]
	for _, directSharedLibraryDependency := range directSharedLibraryDependencies {
		err := loadLibrary(librariesDir, sourcesDir, downloadedSourcesDir, loadedLibraries, loadedSources, directSharedLibraryDependency)
		if err != nil {
			return "", err
		}
	}

	loadedExecutables[sourceName+" "+sourceExecutableRelativePath] = sourceExecutable
	return sourceExecutable, nil
}

func exec(sourceName string, sourceExecutableRelativePath string, bentoDir string, argsToPass []string) {
	libraries := map[string]parsedLibrary{}
	sources := map[string]parsedSourceConfig{}
	executables := map[string]string{}

	executableEnvironmentUnparsed := os.Environ()
	executableEnvironment := map[string]string{}
	for _, environmentVariable := range executableEnvironmentUnparsed {
		environmentVariableSplit := strings.SplitN(environmentVariable, "=", 2)
		executableEnvironment[environmentVariableSplit[0]] = environmentVariableSplit[1]
	}

	sourceExecutable, err := loadExecutable(
		path.Join(bentoDir, "sources"),
		path.Join(bentoDir, "downloadedSources"),
		sources, path.Join(bentoDir, "lib"),
		libraries,
		sourceName,
		sourceExecutableRelativePath,
		executables,
		executableEnvironment,
	)
	if err != nil {
		utils.Fail(err.Error())
	}

	downloads := make([]utils.DownloadOptions, 0, len(sources))
	downloadsSortedByLicense := map[string][][]string{}
	for sourceName, sourceConf := range sources {
		_, err := os.Stat(sourceConf.path)
		if os.IsNotExist(err) {
			downloadsSortedByLicense[sourceConf.licenseDescription] = append(
				downloadsSortedByLicense[sourceConf.licenseDescription],
				append([]string{sourceName}, sourceConf.installationWarnings...),
			)
			downloads = append(downloads, utils.DownloadOptions{
				Name:                             sourceName,
				Urls:                             sourceConf.parsedUrls,
				Compression:                      sourceConf.compression,
				Checksum:                         sourceConf.parsedChecksum,
				UseChecksum:                      true,
				FilesToMakeExecutable:            sourceConf.filesToMakeExecutable,
				RootPath:                         sourceConf.parsedRootPath,
				Destination:                      sourceConf.path,
				DeleteExistingFilesAtDestination: false,
			})
		} else if err != nil {
			utils.Fail("Failed to stat `" + sourceConf.path + "`: " + err.Error())
		}
	}
	if len(downloads) > 0 {
		println("Download the following " + utils.CreateNoun(len(downloads), "source", "sources") + " to run the binary " + sourceExecutableRelativePath + " from the source " + sourceName + "?")
		for licenseHeader, sources := range downloadsSortedByLicense {
			println("- " + utils.AnsiBold + utils.CreateNoun(len(sources), "A source", "sources") + " " + licenseHeader + utils.AnsiReset)
			for _, source := range sources {
				println("  - " + source[0])
				for _, installationWarning := range source[1:] {
					println("    - " + installationWarning)
				}
			}
		}
		if !utils.GetBoolDefaultYes() {
			return
		}
		errs := utils.DownloadConcurrently(downloads, maxParrellelDownloads)
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
