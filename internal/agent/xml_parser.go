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

	type stackItem struct {
		name string
		data map[string]interface{}
	}

	var stack []stackItem
	var currentItem *stackItem
	arrays := make(map[string][]interface{})

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

			// Special handling for array items
			if name == "items" && len(stack) > 0 && stack[len(stack)-1].name == "params" {
				// Starting a new array item
				newItem := stackItem{
					name: name,
					data: make(map[string]interface{}),
				}
				stack = append(stack, newItem)
				currentItem = &stack[len(stack)-1]
			} else if currentItem != nil && currentItem.name == "items" {
				// We're inside an items object, this is a property
				stack = append(stack, stackItem{name: name, data: nil})
			} else if name != "params" {
				// Regular parameter
				stack = append(stack, stackItem{name: name, data: nil})
			} else {
				// This is the params root
				stack = append(stack, stackItem{name: name, data: params})
				currentItem = &stack[len(stack)-1]
			}

		case xml.EndElement:
			name := t.Name.Local

			if len(stack) > 0 {
				if name == "items" && currentItem != nil && currentItem.name == "items" {
					// Ending an array item, add it to the arrays map
					if _, exists := arrays["items"]; !exists {
						arrays["items"] = []interface{}{}
					}
					arrays["items"] = append(arrays["items"], currentItem.data)

					// Pop the item
					stack = stack[:len(stack)-1]
					if len(stack) > 0 {
						currentItem = &stack[len(stack)-1]
					} else {
						currentItem = nil
					}
				} else if name != "params" {
					// Pop regular element
					if len(stack) > 0 {
						stack = stack[:len(stack)-1]
					}
				}
			}

		case xml.CharData:
			value := strings.TrimSpace(string(t))
			if value != "" && len(stack) > 0 {
				current := stack[len(stack)-1]

				// Convert value to appropriate type
				var finalValue interface{} = value

				// Try boolean
				if value == "true" || value == "false" {
					finalValue = value == "true"
				} else if intVal, err := strconv.Atoi(value); err == nil {
					// Try integer
					finalValue = intVal
				} else if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
					// Try float
					finalValue = floatVal
				}

				// Store the value
				if currentItem != nil && currentItem.name == "items" && len(stack) >= 2 {
					// We're inside an items object
					currentItem.data[current.name] = finalValue
				} else if len(stack) >= 2 && stack[len(stack)-2].name == "params" {
					// Direct child of params
					params[current.name] = finalValue
				}
			}
		}
	}

	// Merge arrays into params
	for key, value := range arrays {
		params[key] = value
	}

	logger.Debug("Parsed XML params", "params", params)
	return params, nil
}
