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
	status.setState("fetching      ")
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
	_, err = io.Copy(responseBuffer, responseReader)
	if err != nil {
		return []byte{}, err
	}
	return responseBuffer.Bytes(), nil
}

type DownloadOptions struct {
	Name                             string
	Url                              string
	Compression                      string
	UseChecksum                      bool
	Checksum                         [32]byte
	FilesToMakeExecutable            []string
	RootPath                         string
	Destination                      string
	DeleteExistingFilesAtDestination bool
}

func download(options DownloadOptions, status stateWithNotifier[string], logs chan<- log) {
	response, err := fetch(options.Url, status)
	if err != nil {
		logs <- log{message: "Failed to fetch `" + options.Name + "` from `" + options.Url + "`: " + err.Error(), isError: true}
		status.setState("failed")
		return
	}
	logs <- log{message: "Fetched `" + options.Name + "` from `" + options.Url + "`"}

	if options.UseChecksum {
		status.setState("checking hash")
		dataChecksum := sha256.Sum256(response)
		if dataChecksum != options.Checksum {
			logs <- log{
				message: "Expected sha256 checksum of `" + options.Name + "` to be 0x" + hex.EncodeToString(options.Checksum[:]) + ", but got 0x" + hex.EncodeToString(dataChecksum[:]),
				isError: true,
			}
			status.setState("failed")
			return
		}
		logs <- log{message: "Cryptographically verified `" + options.Name + "` using sha256 hash"}
	}

	if options.DeleteExistingFilesAtDestination {
		status.setState("deleting old files")
		err := os.RemoveAll(options.Destination)
		if err != nil && !os.IsNotExist(err) {
			logs <- log{message: err.Error(), isError: true}
		}
	}

	status.setState("extracting")
	err = extract(response, options.Compression, options.Destination, options.RootPath)
	if err != nil {
		logs <- log{message: "Failed to extract `" + options.Name + "`: " + err.Error(), isError: true}
		status.setState("failed")
		return
	}
	logs <- log{message: "Extracted `" + options.Name + "` into " + options.Destination}

	for i, fileName := range options.FilesToMakeExecutable {
		status.setState(fmt.Sprintf("making files executable (%d/%d)", i+1, len(options.FilesToMakeExecutable)))
		absoluteFileName := path.Join(options.Destination, fileName)
		fileInfo, err := os.Stat(absoluteFileName)
		if err != nil {
			logs <- log{message: "Failed to make the file `" + fileName + "` executable: " + err.Error(), isError: true}
			continue
		}
		err = os.Chmod(absoluteFileName, fileInfo.Mode()|0111)
		if err != nil {
			logs <- log{message: "Failed to make the file `" + fileName + "` executable: " + err.Error(), isError: true}
			continue
		}
		logs <- log{message: "Made `" + absoluteFileName + "` executable"}
	}

	status.setState("done")
}

func DownloadConcurrently(sources []DownloadOptions) []error {
	statuses := make([]string, len(sources))
	statusUpdated := make(chan struct{}, 1)
	logs := make(chan log, 10)
	for i, source := range sources {
		go download(source, stateWithNotifier[string]{state: &statuses[i], notifier: statusUpdated}, logs)
	}

	errs := []error{}
	downloadsFinished := false
	for true {
		if !downloadsFinished || len(statusUpdated) > 0 {
			<-statusUpdated
		}
		print(clearBetweenCursorAndScreenEnd)
		for len(logs) > 0 {
			log := <-logs
			if log.isError {
				// TODO: Cancel other downloads when one download has an error
				os.Stderr.WriteString(log.message + "\n")
				errs = append(errs, errors.New(log.message))
			} else {
				println(log.message)
			}
		}
		if downloadsFinished {
			break
		}
		downloadsFinished = true
		for i, source := range sources {
			if statuses[i] != "done" && statuses[i] != "failed" {
				downloadsFinished = false
			}
			println(source.Name + ": " + statuses[i])
		}
		moveCursorUp(len(sources))
		os.Stdout.Sync()
	}
	return errs
}

func FetchPackageRepository(packageCacheDir string) []error {
	return DownloadConcurrently([]DownloadOptions{{
		Name:                             "Package repository",
		Url:                              "https://github.com/godalming123/binary-repository/archive/refs/heads/main.zip",
		Compression:                      ".zip",
		UseChecksum:                      false,
		RootPath:                         "binary-repository-main",
		Destination:                      packageCacheDir,
		DeleteExistingFilesAtDestination: true,
	}})
}
