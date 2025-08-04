package utils

import "errors"
import "io"
import "os"
import "path"
import "bytes"
import "archive/zip"
import "archive/tar"
import "compress/gzip"
import "compress/bzip2"
import "github.com/ulikunitz/xz"
import "github.com/klauspost/compress/zstd"

func archivePathToSystemPath(pathRelativeToArchiveRoot string, rootPath string, absoluteDestination string) (absolutePath string, inRoot bool) {
	// Use `path.Clean` to stop a path like `ROOT_PATH/../../../../../../` being able to pass the inRoot check
	// SECURITY: This is necersarry to stop compressed files from being able to create directories/files outside the destination
	pathRelativeToDestination, inRoot := TrimPrefix(path.Clean(pathRelativeToArchiveRoot), rootPath)
	if !inRoot {
		return "", false
	}
	return path.Join(absoluteDestination, pathRelativeToDestination), true
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
		filePath, inRoot := archivePathToSystemPath(file.Name, rootPath, destination)
		if !inRoot {
			continue
		}

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

		headerOutputPath, inRoot := archivePathToSystemPath(header.Name, rootPath, destination)
		if !inRoot {
			continue
		}

		switch header.Typeflag {
		case tar.TypeReg:
			err = os.MkdirAll(path.Dir(headerOutputPath), 0755)
			if err != nil {
				return err
			}
			var outFile *os.File
			outFile, err = os.OpenFile(headerOutputPath, os.O_CREATE|os.O_WRONLY, header.FileInfo().Mode().Perm())
			if err != nil {
				return err
			}
			defer outFile.Close()
			_, err = io.Copy(outFile, untarredStream)
		case tar.TypeLink:
			linkOldPath, linkOldPathInRoot := archivePathToSystemPath(header.Linkname, rootPath, destination)
			if !linkOldPathInRoot {
				continue
			}
			err = os.Link(linkOldPath, headerOutputPath)
		case tar.TypeSymlink:
			err = os.Symlink(header.Linkname, headerOutputPath)
		case tar.TypeDir:
			err = os.MkdirAll(headerOutputPath, 0755)
		default:
			return errors.New("Unknown type: " + string([]byte{header.Typeflag}) + " in " + header.Name)
		}
		if err != nil {
			return err
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
	var uncompressedFileStream io.Reader
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
	case ".tar.zst":
		partiallyUncompressedStream, err := zstd.NewReader(stream)
		if err != nil {
			return err
		}
		return extractTar(partiallyUncompressedStream, destination, rootPath)
	case ".tbz":
		partiallyUncompressedStream := bzip2.NewReader(stream)
		return extractTar(partiallyUncompressedStream, destination, rootPath)
	case ".zip":
		return extractZip(stream, destination, rootPath)
	case ".gz":
		var err error
		uncompressedFileStream, err = gzip.NewReader(stream)
		if err != nil {
			return err
		}
	case "none":
		uncompressedFileStream = stream
	default:
		return errors.New("Unknown compression format `" + compressionType + "`. Supported compression formats are `.tar.gz`, `.tar.xz`, `.tar.zst`, `.tbz`, `.zip`, `.gz` and `none`.")
	}
	err := os.MkdirAll(path.Dir(destination), 0755)
	if err != nil {
		return err
	}
	outFile, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer outFile.Close()
	_, err = io.Copy(outFile, uncompressedFileStream)
	return err
}
