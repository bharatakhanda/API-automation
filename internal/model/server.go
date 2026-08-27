package model

// FileSelectionMode controls how test files are selected from a user-provided folder.
type FileSelectionMode string

const (
	FileSelectionAll    FileSelectionMode = "all"
	FileSelectionSingle FileSelectionMode = "single"
	FileSelectionRandom FileSelectionMode = "random"
)

// ServerConnection captures the required server-side execution settings.
type ServerConnection struct {
	IPAddress string `json:"ip_address"`
	SecretKey string `json:"secret_key"`
}

// TestFileSelection captures the local test file source and selection strategy.
type TestFileSelection struct {
	FolderPath string            `json:"folder_path"`
	Mode       FileSelectionMode `json:"mode"`
	FilePath   string            `json:"file_path,omitempty"`
}
