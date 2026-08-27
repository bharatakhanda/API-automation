package files

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"api-automation/internal/model"
)

var supportedJobExtensions = map[string]bool{
	".pdf":  true,
	".ps":   true,
	".eps":  true,
	".prn":  true,
	".tif":  true,
	".tiff": true,
	".jpg":  true,
	".jpeg": true,
	".png":  true,
}

// Select resolves the effective test files based on the requested selection mode.
func Select(selection model.TestFileSelection) ([]string, error) {
	if selection.FolderPath == "" {
		return nil, errors.New("test folder is required")
	}

	files, err := listRegularFiles(selection.FolderPath)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errors.New("test folder does not contain files")
	}

	switch selection.Mode {
	case model.FileSelectionAll, "":
		return files, nil
	case model.FileSelectionSingle:
		if selection.FilePath == "" {
			return nil, errors.New("file path is required for single-file selection")
		}
		cleanFolder, err := filepath.Abs(selection.FolderPath)
		if err != nil {
			return nil, err
		}
		cleanFile, err := filepath.Abs(selection.FilePath)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(cleanFolder, cleanFile)
		if err != nil || rel == ".." || len(rel) >= 3 && rel[:3] == ".."+string(os.PathSeparator) {
			return nil, errors.New("selected file must be inside the test folder")
		}
		info, err := os.Stat(cleanFile)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			return nil, errors.New("selected path is a directory")
		}
		if !isSupportedJobFile(cleanFile) {
			return nil, fmt.Errorf("selected file %q is not a supported Fiery job file; supported extensions: %s", filepath.Base(cleanFile), supportedExtensionList())
		}
		return []string{cleanFile}, nil
	case model.FileSelectionRandom:
		idx, err := secureRandomIndex(len(files))
		if err != nil {
			return nil, err
		}
		return []string{files[idx]}, nil
	default:
		return nil, errors.New("unknown file selection mode")
	}
}

func listRegularFiles(folder string) ([]string, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(folder, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if info.Mode().IsRegular() && isSupportedJobFile(path) {
			abs, err := filepath.Abs(path)
			if err != nil {
				return nil, err
			}
			files = append(files, abs)
		}
	}
	sort.Strings(files)
	return files, nil
}

func isSupportedJobFile(path string) bool {
	return supportedJobExtensions[strings.ToLower(filepath.Ext(path))]
}

func supportedExtensionList() string {
	extensions := make([]string, 0, len(supportedJobExtensions))
	for ext := range supportedJobExtensions {
		extensions = append(extensions, ext)
	}
	sort.Strings(extensions)
	return strings.Join(extensions, ", ")
}

func secureRandomIndex(length int) (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(length)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}
