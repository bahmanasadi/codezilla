package agent

import (
	"encoding/xml"
	"io"
	"strconv"
	"strings"

	"codezilla/pkg/logger"
)

// parseXMLParams parses parameters from XML data with support for arrays
func parseXMLParams(xmlData []byte, logger *logger.Logger) (map[string]interface{}, error) {
	params := make(map[string]interface{})

	// Use a custom XML decoder that handles arrays
	decoder := xml.NewDecoder(strings.NewReader(string(xmlData)))

	// Stack to track nested elements
	var elementStack []string
	var currentKey string

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			logger.Error("Error parsing XML", "error", err)
			return nil, err
		}

		switch t := token.(type) {
		case xml.StartElement:
			name := t.Name.Local
			elementStack = append(elementStack, name)
			currentKey = name

		case xml.EndElement:
			if len(elementStack) > 0 {
				elementStack = elementStack[:len(elementStack)-1]
			}
			if len(elementStack) > 0 {
				currentKey = elementStack[len(elementStack)-1]
			}

		case xml.CharData:
			value := strings.TrimSpace(string(t))
			if value != "" && currentKey != "" {
				// Convert value to appropriate type
				var finalValue interface{} = value

				// Try boolean
				if value == "true" || value == "false" {
					finalValue = value == "true"
				} else if intVal, err := strconv.Atoi(value); err == nil {
					// Try integer
					finalValue = intVal
				} else if floatVal, err := strconv.ParseFloat(value, 64); err == nil && strings.Contains(value, ".") {
					// Try float (only if it contains a decimal point)
					finalValue = floatVal
				}

				// Store the value
				params[currentKey] = finalValue
			}
		}
	}

	logger.Debug("Parsed XML params", "params", params)
	return params, nil
}
