package schema

// ValidationError is a machine-readable validation issue.
type ValidationError struct {
	Line    int    `json:"line,omitempty"`
	Code    string `json:"code"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationResult is the stable output shape used by the CLI and the ingest API.
type ValidationResult struct {
	Valid  bool              `json:"valid"`
	File   string            `json:"file,omitempty"`
	Errors []ValidationError `json:"errors"`
}
