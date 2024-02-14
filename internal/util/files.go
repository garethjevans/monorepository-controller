package util

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/fluxcd/pkg/sourceignore"
	"golang.org/x/mod/sumdb/dirhash"
)

func ListFiles(dir string) ([]string, error) {
	return dirhash.DirFiles(dir, ".")
}

func HashFiles(list []string, dir string) (string, error) {
	return dirhash.Hash1(list, func(name string) (io.ReadCloser, error) {
		return os.Open(filepath.Join(dir, name))
	})
}

func FilterFileList(list []string, include string) []string {
	var domain []string
	patterns := sourceignore.ReadPatterns(strings.NewReader(include), domain)
	matcher := sourceignore.NewDefaultMatcher(patterns, domain)

	var filtered []string
	for _, file := range list {
		fileParts := strings.Split(file, string(filepath.Separator))

		if matcher.Match(fileParts, false) {
			filtered = append(filtered, file)
		}
	}

	return filtered
}

// DownloadFile will download a url to a local file. It's efficient because it will
// write as it downloads and not load the whole file into memory.
func DownloadFile(filepath string, url string) error {
	validURL, err := validate(url)
	if err != nil {
		return err
	}
	// Get the data
	resp, err := http.Get(validURL)
	if err != nil {
		return err
	}

	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	return err
}

func validate(in string) (string, error) {
	if strings.HasPrefix(in, "https://") && strings.HasPrefix(in, "http://") {
		return "", errors.New("invalid url provided, it should have prefix https:// or http://")
	}
	return in, nil
}
