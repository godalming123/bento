package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/ulikunitz/xz"
)

func fail(msgs ...string) {
	for _, msg := range msgs {
		os.Stderr.WriteString(msg)
	}
	os.Stderr.Write([]byte{'\n'})
	os.Exit(1)
}

func trimPrefix(str string, prefix string) (string, bool) {
	if strings.HasPrefix(str, prefix) {
		return str[len(prefix):], true
	}
	return str, false
}

func fetch(url string) ([]byte, error) {
	response, err := http.Get(url)
	if err == nil {
		defer response.Body.Close()
		responseBuffer := bytes.NewBuffer([]byte{})
		_, err := io.Copy(responseBuffer, response.Body)
		if err == nil {
			return responseBuffer.Bytes(), nil
		}
	}
	return []byte{}, errors.New("Failed to fetch `" + url + "`: " + err.Error())
}

func extractZip(
	stream *bytes.Reader,
	destination string,
	rootPath string,
) error {
	unzipped, err := zip.NewReader(stream, int64(stream.Len()))
	if err != nil {
		return err
	}
	for _, file := range unzipped.File {
		// Use `path.Clean` to stop a path like `ROOT_PATH/../../../../../../` being able to pass the inRoot check
		// SECURITY: This is necersarry to stop compressed files from being able to create directories/files outside the destination
		filePath, inRoot := trimPrefix(path.Clean(file.Name), rootPath)
		if !inRoot {
			continue
		}
		filePath = path.Join(destination, filePath)

		if file.FileInfo().IsDir() {
			err := os.MkdirAll(filePath, file.Mode())
			if err != nil {
				return err
			}
		} else {
			err := os.MkdirAll(path.Dir(filePath), 0755)
			if err != nil {
				return err
			}

			zipFile, err := file.Open()
			if err != nil {
				return err
			}
			defer zipFile.Close()

			if file.Mode()&os.ModeSymlink != 0 {
				symlinkTarget, err := io.ReadAll(zipFile)
				if err != nil {
					return err
				}
				err = os.Symlink(string(symlinkTarget), filePath)
				if err != nil {
					return err
				}
			} else {
				destFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
				if err != nil {
					return err
				}
				defer destFile.Close()

				_, err = io.Copy(destFile, zipFile)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func extractTar(
	stream io.Reader,
	destination string,
	rootPath string,
) error {
	untarredStream := tar.NewReader(stream)
	for true {
		header, err := untarredStream.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		if header.Typeflag != tar.TypeDir && header.Typeflag != tar.TypeReg {
			return errors.New("Uknown type: " + string([]byte{header.Typeflag}) + " in " + header.Name)
		}

		// Use `path.Clean` to stop a path like `ROOT_PATH/../../../../../../` being able to pass the inRoot check
		// SECURITY: This is necersarry to stop compressed files from being able to create directories/files outside the destination
		headerOutputPath, inRoot := trimPrefix(path.Clean(header.Name), rootPath)
		if !inRoot {
			continue
		}
		headerOutputPath = path.Join(destination, headerOutputPath)

		switch header.Typeflag {
		case tar.TypeDir:
			err := os.MkdirAll(headerOutputPath, 0755)
			if err != nil {
				return err
			}
		case tar.TypeReg:
			err := os.MkdirAll(path.Dir(headerOutputPath), 0755)
			if err != nil {
				return err
			}
			outFile, err := os.Create(headerOutputPath)
			if err != nil {
				return err
			}
			defer outFile.Close()
			_, err = io.Copy(outFile, untarredStream)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func extract(
	data []byte,
	compressionType string,
	destination string,
	rootPath string,
) error {
	stream := bytes.NewReader(data)
	switch compressionType {
	case ".tar.gz":
		partiallyUncompressedStream, err := gzip.NewReader(stream)
		if err != nil {
			return err
		}
		return extractTar(partiallyUncompressedStream, destination, rootPath)
	case ".tar.xz":
		partiallyUncompressedStream, err := xz.NewReader(stream)
		if err != nil {
			return err
		}
		return extractTar(partiallyUncompressedStream, destination, rootPath)
	case ".zip":
		return extractZip(stream, destination, rootPath)
	case "none":
		outFile, err := os.Create(destination)
		if err != nil {
			return err
		}
		defer outFile.Close()
		_, err = io.Copy(outFile, stream)
		return err
	default:
		return errors.New("Unkown compression format `" + compressionType + "`. Supported compression formats are `.tar.gz`, `.tar.xz`, `.zip`, and `none`.")
	}
}

func fetchPackageRepository(packageCacheDir string) error {
	packageRepoUrl := "https://github.com/godalming123/binary-repository/archive/refs/heads/main.zip"
	println("Fetching package repository from " + packageRepoUrl)
	response, err := fetch(packageRepoUrl)
	if err != nil {
		return errors.New("Failed to fetch `" + packageRepoUrl + "`: " + err.Error())
	}

	println("Removing old package repository from `" + packageCacheDir + "` if there is one")
	err = os.RemoveAll(packageCacheDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	println("Extracting package repository into `" + packageCacheDir + "`")
	err = extractZip(bytes.NewReader(response), packageCacheDir, "binary-repository-main")
	if err != nil {
		return errors.New("Failed to extract package repository: " + err.Error())
	}

	return nil
}
