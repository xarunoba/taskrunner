package task

type FieldType string

const (
	FieldText    FieldType = "text"
	FieldChoice  FieldType = "choice"
	FieldFile    FieldType = "file"
	FieldConfirm FieldType = "confirm"
)

// Field describes one value collected before a task runs.
type Field struct {
	Key      string    `json:"key"`
	Label    string    `json:"label"`
	Type     FieldType `json:"type"`
	Options  []string  `json:"options,omitempty"`
	Raw      bool      `json:"raw,omitempty"`
	Optional bool      `json:"optional,omitempty"`
}

// Task is a shell command and its runtime form.
type Task struct {
	Name    string  `json:"name"`
	Command string  `json:"command"`
	Fields  []Field `json:"fields,omitempty"`
	File    string  `json:"-"`
}
