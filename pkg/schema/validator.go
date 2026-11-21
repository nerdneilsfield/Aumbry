package schema

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"strings"
	"time"
)

// ValidateFile reads an XML file and validates its v1 envelope and metadata.
func ValidateFile(path string) (ValidationResult, error) {
	input, err := os.ReadFile(path)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("read XML file %q: %w", path, err)
	}
	issues := ValidateXML(input)
	if issues == nil {
		issues = []ValidationError{}
	}
	return ValidationResult{
		Valid:  len(issues) == 0,
		File:   path,
		Errors: issues,
	}, nil
}

// ValidateXML checks XML syntax and the required v1 document metadata.
// ValidateXML checks XML syntax and the required v1 document metadata.
func ValidateXML(input []byte) []ValidationError {
	decoder := xml.NewDecoder(bytes.NewReader(input))
	var document Document
	if err := decoder.Decode(&document); err != nil {
		offset := decoder.InputOffset()
		if offset > int64(len(input)) {
			offset = int64(len(input))
		}
		line := bytes.Count(input[:offset], []byte{'\n'}) + 1
		return []ValidationError{{
			Line:    line,
			Code:    "INVALID_XML",
			Field:   "document",
			Message: err.Error(),
		}}
	}

	var issues []ValidationError
	if document.XMLName.Local != "document" {
		issues = append(issues, ValidationError{
			Code: "INVALID_ROOT", Field: "document",
			Message: "root element must be <document>",
		})
	}
	if document.Version != "1.0" {
		issues = append(issues, ValidationError{
			Code: "UNSUPPORTED_VERSION", Field: "document.version",
			Message: "version must be 1.0",
		})
	}

	required := []struct {
		field string
		value string
	}{
		{"meta.title", document.Meta.Title},
		{"meta.type", document.Meta.Type},
		{"meta.date", document.Meta.Date},
		{"meta.summary", document.Meta.Summary},
		{"meta.source-agent", document.Meta.SourceAgent},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			issues = append(issues, ValidationError{
				Code: "REQUIRED_FIELD", Field: field.field,
				Message: fmt.Sprintf("%s is required", field.field),
			})
		}
	}

	if document.Meta.Type != "" && document.Meta.Type != "report" && document.Meta.Type != "post" {
		issues = append(issues, ValidationError{
			Code: "INVALID_TYPE", Field: "meta.type",
			Message: "type must be report or post",
		})
	}
	if document.Meta.Date != "" {
		if _, err := time.Parse("2006-01-02", document.Meta.Date); err != nil {
			issues = append(issues, ValidationError{
				Code: "INVALID_DATE", Field: "meta.date",
				Message: "date must use YYYY-MM-DD format",
			})
		}
	}
	return issues
}
