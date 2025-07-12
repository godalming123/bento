package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"github.com/ulikunitz/xz"
	"io"
	"os"
	"path"
	"strings"
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

func extract(
	stream io.Reader,
	compressionType string,
	destination string,
	rootPath string,
) error {
	var partiallyUncompressedStream io.Reader
	var err error
	switch compressionType {
	case ".tar.gz":
		partiallyUncompressedStream, err = gzip.NewReader(stream)
	case ".tar.xz":
		partiallyUncompressedStream, err = xz.NewReader(stream)
	default:
		return errors.New("Unkown compression format " + compressionType)
	}
	if err != nil {
		return err
	}
	untarredStream := tar.NewReader(partiallyUncompressedStream)
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

		headerOutputPath, inRoot := trimPrefix(header.Name, rootPath)
		if !inRoot {
			continue
		}
		headerOutputPath = path.Join(destination, headerOutputPath)

		switch header.Typeflag {
		case tar.TypeDir:
			err := os.Mkdir(headerOutputPath, 0755)
			if err != nil {
				return err
			}
		case tar.TypeReg:
			outFile, err := os.Create(headerOutputPath)
			if err != nil {
				return err
			}
			_, err = io.Copy(outFile, untarredStream)
			if err != nil {
				return err
			}
			outFile.Close()
		}
	}
	return nil
}
