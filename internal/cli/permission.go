package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"codezilla/internal/tools"
	"codezilla/pkg/style"
)

// cliPermissionCallback prompts the user for permission to execute a tool
// and returns their response.
func cliPermissionCallback(in io.Reader, out io.Writer, request tools.PermissionRequest) (tools.PermissionResponse, error) {
	reader := bufio.NewReader(in)

	// Format the request details
	fmt.Fprint(out, style.ColorBold(style.ColorCodeYellow, "\n🔒 PERMISSION REQUEST 🔒\n"))
	fmt.Fprintf(out, "Tool: %s\n", style.ColorBold(style.ColorCodeCyan, request.ToolContext.ToolName))
	fmt.Fprintf(out, "Action: %s\n", style.ColorBold(style.ColorCodeWhite, request.Description))

	// Check if there's a file diff to display
	fileDiff, hasDiff := request.ToolContext.Params["_fileDiff"].(string)
	filePath, _ := request.ToolContext.Params["file_path"].(string)

	// Debug info - write to stderr to avoid interfering with UI
	fmt.Fprintf(os.Stderr, "\nDEBUG: ToolName=%s, hasDiff=%v, fileDiff empty=%v\n",
		request.ToolContext.ToolName, hasDiff, fileDiff == "")

	// If this is a fileWrite operation with a diff, show the diff
	if request.ToolContext.ToolName == "fileWrite" && hasDiff && fileDiff != "" {
		fmt.Fprintln(out, "")
		fmt.Fprintf(out, style.ColorBold(style.ColorCodeWhite, "Changes to %s:\n"), style.ColorBold(style.ColorCodeCyan, filePath))
		fmt.Fprintln(out, fileDiff)
	} else {
		// Format regular parameters for display
		fmt.Fprintln(out, "Parameters:")
		for k, v := range request.ToolContext.Params {
			// Skip internal parameters that start with _
			if !strings.HasPrefix(k, "_") {
				fmt.Fprintf(out, "  %s: %v\n", k, v)
			}
		}
	}

	// Ask for permission with options
	for {
		fmt.Fprintln(out, "")
		fmt.Fprintf(out, "Allow this operation? [%s/%s/%s/%s] ",
			style.ColorGreen("y=yes"),
			style.ColorRed("n=no"),
			style.ColorBlue("a=always for this tool"),
			style.ColorYellow("d=deny always for this tool"))

		// Read user input
		input, err := reader.ReadString('\n')
		if err != nil {
			return tools.PermissionResponse{Granted: false}, fmt.Errorf("failed to read input: %w", err)
		}

		// Process response
		input = strings.TrimSpace(strings.ToLower(input))
		switch input {
		case "y", "yes":
			return tools.PermissionResponse{Granted: true, RememberMe: false}, nil
		case "n", "no":
			return tools.PermissionResponse{Granted: false, RememberMe: false}, nil
		case "a", "always":
			// Set permission level for this tool to NeverAsk
			if request.Tool != nil {
				fmt.Fprintf(out, "✓ Will always allow operations for tool '%s'\n", request.ToolContext.ToolName)
				// Set the permission level to NeverAsk for this tool
				return tools.PermissionResponse{Granted: true, RememberMe: true}, nil
			}
			return tools.PermissionResponse{Granted: true, RememberMe: true}, nil
		case "d", "deny":
			// Set permission level for this tool to AlwaysDeny
			if request.Tool != nil {
				fmt.Fprintf(out, "✗ Will always deny operations for tool '%s'\n", request.ToolContext.ToolName)
				return tools.PermissionResponse{Granted: false, RememberMe: true}, nil
			}
			return tools.PermissionResponse{Granted: false, RememberMe: true}, nil
		default:
			fmt.Fprintf(out, "Invalid option. Please enter 'y', 'n', 'a', or 'd'.\n")
		}
	}
}

// handlePermissionsCommand handles the /permissions command
func (a *App) handlePermissionsCommand(args string) error {
	if args == "" {
		// Display current permission settings
		return a.listPermissions()
	}

	// Special command to toggle always ask mode
	if args == "always-ask" {
		// Toggle the always ask setting
		a.Config.AlwaysAskPermission = !a.Config.AlwaysAskPermission

		// Save the config
		if err := SaveConfig(a.Config, "config.json"); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		status := "enabled"
		if !a.Config.AlwaysAskPermission {
			status = "disabled"
		}

		fmt.Fprintf(a.Writer, "Always ask for permission mode %s\n",
			style.ColorBold(style.ColorCodeGreen, status))

		return nil
	}

	// Parse arguments: /permissions <tool> <level>
	parts := strings.SplitN(args, " ", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid permissions command format. Use /permissions <tool> <level> or /permissions always-ask")
	}

	tool := parts[0]
	level := strings.ToLower(parts[1])

	// Validate tool exists
	_, found := a.ToolRegistry.GetTool(tool)
	if !found {
		return fmt.Errorf("tool not found: %s", tool)
	}

	// Validate permission level
	permLevel, err := validatePermissionLevel(level)
	if err != nil {
		return err
	}

	// Update permission level
	a.Config.ToolPermissions[tool] = permLevel

	// Save the updated configuration
	if err := SaveConfig(a.Config, "config.json"); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// We'll store the permission level in the config
	// It will be loaded by the permission manager on next execution

	// We can't directly access the agent's permission manager due to interface abstraction
	// The permission settings will be applied on next tool execution via the config

	fmt.Fprintf(a.Writer, "Permission level for '%s' set to: %s\n",
		style.ColorBold(style.ColorCodeCyan, tool),
		style.ColorBold(style.ColorCodeGreen, level))

	return nil
}

// listPermissions displays current permission settings
func (a *App) listPermissions() error {
	fmt.Fprintln(a.Writer, style.ColorBold(style.ColorCodeWhite, "Current permission settings:"))

	// Default warning level
	fmt.Fprintf(a.Writer, "Warn on dangerous tools: %s\n",
		formatBool(a.Config.DangerousToolsWarn))

	// Tool-specific permissions
	fmt.Fprintln(a.Writer, "\nTool permissions:")

	// First list tools with explicit permissions
	hasExplicitPerms := false
	for name, level := range a.Config.ToolPermissions {
		// Check if the tool still exists
		_, found := a.ToolRegistry.GetTool(name)
		if !found {
			// Skip tools that no longer exist
			continue
		}

		hasExplicitPerms = true
		fmt.Fprintf(a.Writer, "  %s: %s\n",
			style.ColorBold(style.ColorCodeCyan, name),
			formatPermissionLevel(level))
	}

	if !hasExplicitPerms {
		fmt.Fprintln(a.Writer, "  No explicit permissions set")
	}

	// Then list tools with default permissions
	fmt.Fprintln(a.Writer, "\nDefault permissions for other tools:")
	fmt.Fprintf(a.Writer, "  %s: %s\n",
		style.ColorBold(style.ColorCodeCyan, "execute"),
		formatPermissionLevel("always_ask"))
	fmt.Fprintf(a.Writer, "  %s: %s\n",
		style.ColorBold(style.ColorCodeCyan, "fileWrite"),
		formatPermissionLevel("always_ask"))
	fmt.Fprintf(a.Writer, "  %s: %s\n",
		style.ColorBold(style.ColorCodeCyan, "fileRead"),
		formatPermissionLevel("ask_once"))
	fmt.Fprintf(a.Writer, "  %s: %s\n",
		style.ColorBold(style.ColorCodeCyan, "other tools"),
		formatPermissionLevel("always_ask"))

	// Display always-ask status
	fmt.Fprintf(a.Writer, "\nAlways ask for permission: %s\n",
		formatPermissionStatus(a.Config.AlwaysAskPermission))

	// Usage information
	fmt.Fprintln(a.Writer, "\nUsage:")
	fmt.Fprintln(a.Writer, "  /permissions <tool> <level>   - Set permission level for a tool")
	fmt.Fprintln(a.Writer, "  /permissions always-ask       - Toggle always-ask mode (ignores saved permissions)")
	fmt.Fprintln(a.Writer, "  Valid levels: never_ask, ask_once, always_ask, always_deny")

	return nil
}

// validatePermissionLevel validates and normalizes a permission level string
func validatePermissionLevel(level string) (string, error) {
	level = strings.ToLower(level)

	switch level {
	case "never_ask", "never", "trust", "always_allow":
		return "never_ask", nil
	case "ask_once", "once":
		return "ask_once", nil
	case "always_ask", "ask", "prompt":
		return "always_ask", nil
	case "always_deny", "deny", "never_allow", "blocked":
		return "always_deny", nil
	default:
		return "", fmt.Errorf("invalid permission level: %s. Valid levels: never_ask, ask_once, always_ask, always_deny", level)
	}
}

// formatPermissionLevel returns a colorized string for a permission level
func formatPermissionLevel(level string) string {
	switch level {
	case "never_ask", "never", "trust", "always_allow":
		return style.ColorGreen("Always allow (never ask)")
	case "ask_once", "once":
		return style.ColorBlue("Ask once per unique action")
	case "always_ask", "ask", "prompt":
		return style.ColorYellow("Always ask")
	case "always_deny", "deny", "never_allow", "blocked":
		return style.ColorRed("Always deny")
	default:
		return level
	}
}

// formatBool returns a colorized string for a boolean value
func formatBool(val bool) string {
	if val {
		return style.ColorGreen("Enabled")
	}
	return style.ColorRed("Disabled")
}

// formatPermissionStatus returns a colorized string for a permission status
func formatPermissionStatus(val bool) string {
	if val {
		return style.ColorGreen("Enabled (will always prompt for permission)")
	}
	return style.ColorYellow("Disabled (uses saved permission preferences)")
}
