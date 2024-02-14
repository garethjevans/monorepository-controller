package util

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ExtractTarGz(tarGzPath string, dir string) error {
	r, err := os.Open(tarGzPath)
	if err != nil {
		return err
	}

	uncompressedStream, err := gzip.NewReader(r)
	if err != nil {
		return err
	}

	tarReader := tar.NewReader(uncompressedStream)

	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			p, err := SanitizeArchivePath(dir, header.Name)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(p, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			p, err := SanitizeArchivePath(dir, header.Name)
			if err != nil {
				return err
			}
			outFile, err := os.Create(p)
			if err != nil {
				return err
			}
			for {
				_, err := io.CopyN(outFile, tarReader, 1024)
				if err != nil {
					if err == io.EOF {
						break
					}
					return err
				}
			}
			outFile.Close()

		default:
			return err
		}
	}
	return nil
}

// Sanitize archive file pathing from "G305: Zip Slip vulnerability".
func SanitizeArchivePath(d, t string) (v string, err error) {
	v = filepath.Join(d, t)
	if strings.HasPrefix(v, filepath.Clean(d)) {
		return v, nil
	}

	return "", fmt.Errorf("%s: %s", "content filepath is tainted", t)
}
