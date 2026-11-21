package schema

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// ExtractPlainText returns normalized character data for full-text indexing.
func ExtractPlainText(input io.Reader) (string, error) {
	decoder := xml.NewDecoder(input)
	var text strings.Builder
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return strings.Join(strings.Fields(text.String()), " "), nil
		}
		if err != nil {
			return "", fmt.Errorf("decode XML text: %w", err)
		}
		if value, ok := token.(xml.CharData); ok {
			text.Write([]byte(value))
			text.WriteByte(' ')
		}
	}
}
