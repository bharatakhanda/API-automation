package capabilities

// Model is the normalized capability set used by the desktop UI. It hides the
// raw Fiery response shape from presentation and automation code.
type Model struct {
	ServerName   string
	SerialNumber string
	Version      string
	Queues       []Queue
	Options      []Option
}

type Queue struct {
	ID        string
	Name      string
	Available bool
	Editable  bool
}

type Option struct {
	ID      string
	Label   string
	Value   string
	Values  []string
	Scopes  []string
	Enabled bool
}

func (m Model) OptionByID(id string) (Option, bool) {
	for _, option := range m.Options {
		if option.ID == id {
			return option, true
		}
	}
	return Option{}, false
}
