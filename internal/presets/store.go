package presets

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const schemaVersion = 1

type Preset struct {
	Name           string              `json:"name"`
	UpdatedAt      time.Time           `json:"updatedAt"`
	ServerName     string              `json:"serverName,omitempty"`
	ServerSerial   string              `json:"serverSerial,omitempty"`
	SelectedValues map[string][]string `json:"selectedValues,omitempty"`
	NumericInputs  map[string]string   `json:"numericInputs,omitempty"`
	Strategy       string              `json:"strategy"`
	MaxCases       string              `json:"maxCases"`
	ParallelJobs   string              `json:"parallelJobs"`
	RunModes       []string            `json:"runModes,omitempty"`
	FileMode       string              `json:"fileMode,omitempty"`
}

type Store struct {
	path string
}

type fileFormat struct {
	Version int      `json:"version"`
	Presets []Preset `json:"presets"`
}

func New(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("preset path is required")
	}
	return &Store{path: path}, nil
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user configuration directory: %w", err)
	}
	return filepath.Join(dir, "API Automation", "presets.json"), nil
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Store) List() ([]Preset, error) {
	file, err := s.read()
	if err != nil {
		return nil, err
	}
	out := append([]Preset(nil), file.Presets...)
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

func (s *Store) Save(preset Preset) error {
	name, err := validName(preset.Name)
	if err != nil {
		return err
	}
	file, err := s.read()
	if err != nil {
		return err
	}
	preset.Name = name
	preset.UpdatedAt = time.Now().UTC()
	preset.SelectedValues = cloneSelections(preset.SelectedValues)
	preset.NumericInputs = cloneStrings(preset.NumericInputs)
	preset.RunModes = append([]string(nil), preset.RunModes...)

	replaced := false
	for index := range file.Presets {
		if strings.EqualFold(file.Presets[index].Name, name) {
			file.Presets[index] = preset
			replaced = true
			break
		}
	}
	if !replaced {
		file.Presets = append(file.Presets, preset)
	}
	sort.Slice(file.Presets, func(i, j int) bool {
		return strings.ToLower(file.Presets[i].Name) < strings.ToLower(file.Presets[j].Name)
	})
	return s.write(file)
}

func (s *Store) Delete(name string) error {
	name, err := validName(name)
	if err != nil {
		return err
	}
	file, err := s.read()
	if err != nil {
		return err
	}
	filtered := file.Presets[:0]
	found := false
	for _, preset := range file.Presets {
		if strings.EqualFold(preset.Name, name) {
			found = true
			continue
		}
		filtered = append(filtered, preset)
	}
	if !found {
		return fmt.Errorf("preset %q was not found", name)
	}
	file.Presets = filtered
	return s.write(file)
}

func (s *Store) read() (fileFormat, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return fileFormat{}, errors.New("preset store is unavailable")
	}
	body, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return fileFormat{Version: schemaVersion}, nil
	}
	if err != nil {
		return fileFormat{}, fmt.Errorf("read presets: %w", err)
	}
	var file fileFormat
	if err := json.Unmarshal(body, &file); err != nil {
		return fileFormat{}, fmt.Errorf("decode presets: %w", err)
	}
	if file.Version != schemaVersion {
		return fileFormat{}, fmt.Errorf("unsupported preset schema version %d", file.Version)
	}
	return file, nil
}

func (s *Store) write(file fileFormat) error {
	file.Version = schemaVersion
	body, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encode presets: %w", err)
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create preset directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".presets-*.json")
	if err != nil {
		return fmt.Errorf("create temporary preset file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(body); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary presets: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary presets: %w", err)
	}
	if err := os.Rename(tempPath, s.path); err != nil {
		// Windows cannot atomically replace an existing destination with Rename.
		backup := s.path + ".bak"
		_ = os.Remove(backup)
		if moveErr := os.Rename(s.path, backup); moveErr != nil && !errors.Is(moveErr, os.ErrNotExist) {
			return fmt.Errorf("prepare preset replacement: %w", moveErr)
		}
		if moveErr := os.Rename(tempPath, s.path); moveErr != nil {
			_ = os.Rename(backup, s.path)
			return fmt.Errorf("replace preset file: %w", moveErr)
		}
		_ = os.Remove(backup)
	}
	return nil
}

func validName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("preset name is required")
	}
	if utf8.RuneCountInString(name) > 80 {
		return "", errors.New("preset name must be 80 characters or fewer")
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "", errors.New("preset name contains a control character")
		}
	}
	return name, nil
}

func cloneSelections(source map[string][]string) map[string][]string {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string][]string, len(source))
	for key, values := range source {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func cloneStrings(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]string, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
