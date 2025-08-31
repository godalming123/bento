package utils

import "bytes"
import "crypto/sha256"
import "encoding/hex"
import "errors"
import "fmt"
import "io"
import "net/http"
import "os"
import "path"
import "strconv"

func fetch(url string, status stateWithNotifier[string]) ([]byte, error) {
	status.setState("fetching")
	response, err := http.Get(url)
	if err != nil {
		return []byte{}, err
	}
	defer response.Body.Close()

	responseReader := response.Body
	contentLength := response.Header.Get("Content-Length")
	if contentLength != "" {
		var length int64
		length, err = strconv.ParseInt(contentLength, 10, 64)
		if err != nil {
			return []byte{}, err
		}
		responseReader = &progressReader{
			progress{int(length), 0},
			response.Body,
			func(p progress) {
				status.setState(fmt.Sprintf("fetching (%3d%%)", (p.contentReadInBytes*100)/p.contentLengthInBytes))
			},
		}
	}

	responseBuffer := bytes.NewBuffer([]byte{})
	// TODO: Add a timeout (something to stop bento from trying to fetch the URL
	// after a certain amount of time in which no data is received)
	_, err = io.Copy(responseBuffer, responseReader)
	if err != nil {
		return []byte{}, err
	}
	return responseBuffer.Bytes(), nil
}

type DownloadOptions struct {
	Name                             string
	Urls                             []string
	Compression                      string
	UseChecksum                      bool
	Checksum                         [32]byte
	FilesToMakeExecutable            []string
	RootPath                         string
	Destination                      string
	DeleteExistingFilesAtDestination bool
}

func download(options DownloadOptions, status stateWithNotifier[string], logs chan<- log) {
	for _, url := range options.Urls {
		response, err := fetch(url, status)
		if err != nil {
			logs <- nonFatalError("Failed to fetch `" + options.Name + "` from `" + url + "`: " + err.Error())
			continue
		}
		logs <- info("Fetched `" + options.Name + "` from `" + url + "`")

		if options.UseChecksum {
			status.setState("checking hash")
			dataChecksum := sha256.Sum256(response)
			if dataChecksum != options.Checksum {
				logs <- nonFatalError("Expected sha256 checksum of `" + options.Name + "` to be 0x" + hex.EncodeToString(options.Checksum[:]) + ", but got 0x" + hex.EncodeToString(dataChecksum[:]))
				continue
			}
			logs <- log{message: "Cryptographically verified `" + options.Name + "` using sha256 hash"}
		}

		if options.DeleteExistingFilesAtDestination {
			status.setState("deleting old files")
			err := os.RemoveAll(options.Destination)
			if err != nil && !os.IsNotExist(err) {
				logs <- fatalError(err.Error())
			}
		}

		status.setState("extracting")
		err = extract(response, options.Compression, options.Destination, options.RootPath)
		if err != nil {
			logs <- fatalError("Failed to extract `" + options.Name + "`: " + err.Error())
			status.setState("failed")
			return
		}
		logs <- info("Extracted `" + options.Name + "` into " + options.Destination)

		for i, fileName := range options.FilesToMakeExecutable {
			status.setState(fmt.Sprintf("making files executable (%d/%d)", i+1, len(options.FilesToMakeExecutable)))
			absoluteFileName := path.Join(options.Destination, fileName)
			fileInfo, err := os.Stat(absoluteFileName)
			if err != nil {
				logs <- fatalError("Failed to make the file `" + fileName + "` executable: " + err.Error())
				continue
			}
			err = os.Chmod(absoluteFileName, fileInfo.Mode()|0111)
			if err != nil {
				logs <- fatalError("Failed to make the file `" + fileName + "` executable: " + err.Error())
				continue
			}
			logs <- info("Made `" + absoluteFileName + "` executable")
		}

		status.setState("done")
		return
	}
	logs <- fatalError(fmt.Sprintf("Tried fetching `%s` from all %d URLs, but none worked", options.Name, len(options.Urls)))
	status.setState("failed")
}

func DownloadConcurrently(sources []DownloadOptions, maxParrallelDownloads uint) []error {
	statuses := make([]string, len(sources))
	for index := range statuses {
		statuses[index] = "queued"
	}
	statusUpdated := make(chan struct{}, 1)
	logs := make(chan log, 10)

	errs := []error{}
	startedDownloads := 0
	downloadsInProgress := uint(0)
	for true {
		for downloadsInProgress < maxParrallelDownloads && startedDownloads < len(sources) {
			go download(sources[startedDownloads], stateWithNotifier[string]{state: &statuses[startedDownloads], notifier: statusUpdated}, logs)
			startedDownloads += 1
			downloadsInProgress += 1
		}
		if downloadsInProgress > 0 || len(statusUpdated) > 0 {
			<-statusUpdated
		}
		print(clearBetweenCursorAndScreenEnd)
		for len(logs) > 0 {
			log := <-logs
			if log.severity >= nonFatalErrorSeverity {
				os.Stderr.WriteString(log.message + "\n")
				if log.severity == fatalErrorSeverity {
					// TODO: Cancel other downloads when one download has a fatal error
					errs = append(errs, errors.New(log.message))
				}
			} else {
				println(log.message)
			}
		}
		if downloadsInProgress == 0 {
			break
		}
		downloadsInProgress = 0
		for i, source := range sources {
			if statuses[i] != "done" && statuses[i] != "failed" && statuses[i] != "queued" {
				downloadsInProgress += 1
			}
			println(source.Name + ": " + statuses[i])
		}
		moveCursorUp(len(sources))
		os.Stdout.Sync()
	}
	return errs
}

func FetchPackageRepository(packageCacheDir string, maxParrallelDownloads uint) []error {
	return DownloadConcurrently([]DownloadOptions{{
		Name:                             "Package repository",
		Urls:                             []string{"https://github.com/godalming123/binary-repository/archive/refs/heads/main.zip"},
		Compression:                      ".zip",
		UseChecksum:                      false,
		RootPath:                         "binary-repository-main",
		Destination:                      packageCacheDir,
		DeleteExistingFilesAtDestination: true,
	}}, maxParrallelDownloads)
}
