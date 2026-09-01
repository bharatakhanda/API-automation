package capabilities

import "api-automation/internal/fiery"

// Model is the normalized capability set used by the desktop UI. It hides the
// raw Fiery response shape from presentation and automation code.
type Model struct {
	ServerName    string
	SerialNumber  string
	Version       string
	Queues        []Queue
	ServerPresets []fiery.ServerPreset
	Options       []Option
}

type Queue struct {
	ID        string
	Name      string
	Available bool
	Editable  bool
}

type NumericRange struct {
	Min       float64
	Max       float64
	Increment float64
	Precision int
}

// Constraints maps a selected value to dependent option IDs and the compatible
// values reported by Fiery for each dependency.
type Constraints map[string]map[string][]string

type Option struct {
	ID          string
	Label       string
	Group       string
	PPDType     string
	Value       string
	Values      []string
	Scopes      []string
	Enabled     bool
	Editable    bool
	Numeric     bool
	Length      int
	Range       *NumericRange
	Constraints Constraints
	Synthetic   bool
}

func (m Model) OptionByID(id string) (Option, bool) {
	for _, option := range m.Options {
		if option.ID == id {
			return option, true
		}
	}
	return Option{}, false
}
