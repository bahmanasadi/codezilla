package tools

import (
	"context"
	"testing"
)

func TestPermissionManager(t *testing.T) {
	tests := []struct {
		name           string
		toolName       string
		defaultPerm    string
		userResponse   PermissionResponse
		expectCallback bool
	}{
		{
			name:           "Never ask permission",
			toolName:       "fileRead",
			defaultPerm:    "never_ask",
			userResponse:   PermissionResponse{},
			expectCallback: false,
		},
		{
			name:           "Always ask permission - granted",
			toolName:       "fileWrite",
			defaultPerm:    "always_ask",
			userResponse:   PermissionResponse{Granted: true, RememberMe: false},
			expectCallback: true,
		},
		{
			name:           "Always ask permission - denied",
			toolName:       "execute",
			defaultPerm:    "always_ask",
			userResponse:   PermissionResponse{Granted: false, RememberMe: false},
			expectCallback: true,
		},
		{
			name:           "Remember permission",
			toolName:       "fileWrite",
			defaultPerm:    "always_ask",
			userResponse:   PermissionResponse{Granted: true, RememberMe: true},
			expectCallback: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callbackCalled := false
			pm := NewPermissionManager(func(ctx context.Context, req PermissionRequest) (PermissionResponse, error) {
				callbackCalled = true

				// Verify request structure
				if req.ToolContext.ToolName != tt.toolName {
					t.Errorf("Expected tool name %s, got %s", tt.toolName, req.ToolContext.ToolName)
				}

				return tt.userResponse, nil
			})

			// Set default permission
			pm.SetDefaultPermission(tt.toolName, tt.defaultPerm)

			// Create context
			ctx := context.Background()
			toolCtx := &ToolContext{
				ToolName: tt.toolName,
			}

			// Request permission
			req := PermissionRequest{
				Description: "Test action",
				ToolContext: toolCtx,
			}

			resp, err := pm.RequestPermission(ctx, req)
			if err != nil {
				t.Fatalf("RequestPermission failed: %v", err)
			}

			// Verify callback was called as expected
			if callbackCalled != tt.expectCallback {
				t.Errorf("Callback called = %v, expected %v", callbackCalled, tt.expectCallback)
			}

			// Verify response based on permission type
			if tt.defaultPerm == "never_ask" {
				if !resp.Granted {
					t.Error("Expected permission to be granted for 'never_ask'")
				}
			} else if tt.expectCallback {
				if resp.Granted != tt.userResponse.Granted {
					t.Errorf("Expected granted = %v, got %v", tt.userResponse.Granted, resp.Granted)
				}
			}

			// Test remember functionality
			if tt.userResponse.RememberMe && resp.Granted {
				// Request permission again - should not call callback
				callbackCalled = false
				resp2, err := pm.RequestPermission(ctx, req)
				if err != nil {
					t.Fatalf("Second RequestPermission failed: %v", err)
				}

				if callbackCalled {
					t.Error("Callback was called again after 'remember me'")
				}

				if !resp2.Granted {
					t.Error("Expected permission to be granted from memory")
				}
			}
		})
	}
}

func TestDefaultPermissions(t *testing.T) {
	pm := NewPermissionManager(nil)

	// Test default permissions from config
	expectedDefaults := map[string]string{
		"fileRead":      "never_ask",
		"fileReadBatch": "never_ask",
		"listFiles":     "never_ask",
		"projectScan":   "never_ask",
		"fileWrite":     "always_ask",
		"execute":       "always_ask",
	}

	// These should be set by the app when loading config
	for tool, expected := range expectedDefaults {
		pm.SetDefaultPermission(tool, expected)

		// Verify it was set correctly
		if pm.defaultPermissions[tool] != expected {
			t.Errorf("Tool %s: expected %s, got %s", tool, expected, pm.defaultPermissions[tool])
		}
	}
}
