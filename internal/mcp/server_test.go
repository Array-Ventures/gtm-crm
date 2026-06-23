package mcp_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Array-Ventures/gtm-crm/internal/db"
	crmmcp "github.com/Array-Ventures/gtm-crm/internal/mcp"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestServer(t *testing.T) *server.MCPServer {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return crmmcp.NewServer(d, "test")
}

func callTool(t *testing.T, s *server.MCPServer, name string, args map[string]any) gomcp.JSONRPCMessage {
	t.Helper()
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      name,
			"arguments": args,
		},
	}
	raw, err := json.Marshal(req)
	require.NoError(t, err)

	// Need to initialize first
	initReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      0,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "test",
				"version": "1.0",
			},
		},
	}
	initRaw, _ := json.Marshal(initReq)
	s.HandleMessage(context.Background(), initRaw)

	return s.HandleMessage(context.Background(), raw)
}

func TestServerCreation(t *testing.T) {
	s := setupTestServer(t)
	assert.NotNil(t, s)

	// Verify tools are registered
	tools := s.ListTools()
	assert.Contains(t, toolNames(tools), "crm_person_search")
	assert.Contains(t, toolNames(tools), "crm_person_create")
	assert.Contains(t, toolNames(tools), "crm_stats")
	assert.Contains(t, toolNames(tools), "crm_context")
}

func toolNames(tools map[string]*server.ServerTool) []string {
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	return names
}

func TestToolCount(t *testing.T) {
	s := setupTestServer(t)
	tools := s.ListTools()
	// Exact count so an accidental tool deletion is caught. Bump this when adding a tool.
	assert.Equal(t, 33, len(tools), "unexpected number of registered MCP tools")
}

func TestPersonCreateViaMessage(t *testing.T) {
	s := setupTestServer(t)

	resp := callTool(t, s, "crm_person_create", map[string]any{
		"first_name": "Jane",
		"last_name":  "Smith",
	})

	// Parse response
	raw, err := json.Marshal(resp)
	require.NoError(t, err)

	var result struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(raw, &result))
	assert.False(t, result.Result.IsError)

	var person map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.Result.Content[0].Text), &person))
	assert.Equal(t, "Jane", person["first_name"])
	assert.Equal(t, "Smith", person["last_name"])
}

// parseToolResult is a helper that unmarshals an MCPServer HandleMessage response
// into the text content and isError flag, mirroring TestPersonCreateViaMessage.
func parseToolResult(t *testing.T, resp gomcp.JSONRPCMessage) (map[string]any, bool) {
	t.Helper()
	raw, err := json.Marshal(resp)
	require.NoError(t, err)

	var envelope struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(raw, &envelope))

	if envelope.Result.IsError {
		return nil, true
	}
	require.NotEmpty(t, envelope.Result.Content, "expected at least one content item")
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(envelope.Result.Content[0].Text), &payload))
	return payload, false
}

// TestOrgCreateAndUpdate exercises crm_org_create then crm_org_update.
func TestOrgCreateAndUpdate(t *testing.T) {
	s := setupTestServer(t)

	// Create org with a github_url
	createResp := callTool(t, s, "crm_org_create", map[string]any{
		"name":       "Acme Corp",
		"github_url": "https://github.com/acme",
	})
	org, isErr := parseToolResult(t, createResp)
	require.False(t, isErr, "crm_org_create should not return an error")
	assert.Equal(t, "Acme Corp", org["name"])
	orgID, ok := org["id"].(float64)
	require.True(t, ok, "org id should be a number")

	// Update the org name
	updateResp := callTool(t, s, "crm_org_update", map[string]any{
		"id":   orgID,
		"name": "Acme Inc",
	})
	updated, isErr := parseToolResult(t, updateResp)
	require.False(t, isErr, "crm_org_update should not return an error")
	assert.Equal(t, "Acme Inc", updated["name"])
	assert.Equal(t, orgID, updated["id"])
}

// TestOrgCreateIdempotentGitHubURL verifies that creating an org twice with the
// same github_url returns the same org id and no error.
func TestOrgCreateIdempotentGitHubURL(t *testing.T) {
	s := setupTestServer(t)

	args := map[string]any{
		"name":       "Dedupe Org",
		"github_url": "https://github.com/dedupe-org",
	}

	first, isErr := parseToolResult(t, callTool(t, s, "crm_org_create", args))
	require.False(t, isErr, "first crm_org_create should not return an error")
	firstID, ok := first["id"].(float64)
	require.True(t, ok, "first org id should be a number")

	second, isErr := parseToolResult(t, callTool(t, s, "crm_org_create", args))
	require.False(t, isErr, "second crm_org_create should not return an error")
	secondID, ok := second["id"].(float64)
	require.True(t, ok, "second org id should be a number")

	assert.Equal(t, firstID, secondID, "both creates should return the same org id")
}

// TestSignalCreateAndGet exercises crm_signal_create then crm_signal_get.
func TestSignalCreateAndGet(t *testing.T) {
	s := setupTestServer(t)

	createResp := callTool(t, s, "crm_signal_create", map[string]any{
		"signal_type": "github",
		"description": "New repo starred",
	})
	created, isErr := parseToolResult(t, createResp)
	require.False(t, isErr, "crm_signal_create should not return an error")
	assert.Equal(t, "github", created["signal_type"])
	sigID, ok := created["id"].(float64)
	require.True(t, ok, "signal id should be a number")

	getResp := callTool(t, s, "crm_signal_get", map[string]any{
		"id": sigID,
	})
	fetched, isErr := parseToolResult(t, getResp)
	require.False(t, isErr, "crm_signal_get should not return an error")
	assert.Equal(t, sigID, fetched["id"])
	assert.Equal(t, "github", fetched["signal_type"])
}

// TestDealCreateAndGet exercises crm_deal_create then crm_deal_get.
func TestDealCreateAndGet(t *testing.T) {
	s := setupTestServer(t)

	createResp := callTool(t, s, "crm_deal_create", map[string]any{
		"title": "Series A",
		"stage": "prospect",
	})
	created, isErr := parseToolResult(t, createResp)
	require.False(t, isErr, "crm_deal_create should not return an error")
	assert.Equal(t, "Series A", created["title"])
	dealID, ok := created["id"].(float64)
	require.True(t, ok, "deal id should be a number")

	getResp := callTool(t, s, "crm_deal_get", map[string]any{
		"id": dealID,
	})
	fetched, isErr := parseToolResult(t, getResp)
	require.False(t, isErr, "crm_deal_get should not return an error")
	assert.Equal(t, dealID, fetched["id"])
	assert.Equal(t, "Series A", fetched["title"])
}

// TestTaskCreateAndGet exercises crm_task_create then crm_task_get.
func TestTaskCreateAndGet(t *testing.T) {
	s := setupTestServer(t)

	createResp := callTool(t, s, "crm_task_create", map[string]any{
		"title":    "Follow up with investor",
		"priority": "high",
	})
	created, isErr := parseToolResult(t, createResp)
	require.False(t, isErr, "crm_task_create should not return an error")
	assert.Equal(t, "Follow up with investor", created["title"])
	taskID, ok := created["id"].(float64)
	require.True(t, ok, "task id should be a number")

	getResp := callTool(t, s, "crm_task_get", map[string]any{
		"id": taskID,
	})
	fetched, isErr := parseToolResult(t, getResp)
	require.False(t, isErr, "crm_task_get should not return an error")
	assert.Equal(t, taskID, fetched["id"])
	assert.Equal(t, "Follow up with investor", fetched["title"])
}

// TestPersonCreateWithGitHubURL verifies that a person can be created with a
// github_url and that the field is reflected in the response.
func TestPersonCreateWithGitHubURL(t *testing.T) {
	s := setupTestServer(t)

	resp := callTool(t, s, "crm_person_create", map[string]any{
		"first_name": "Alice",
		"github_url": "https://github.com/alice",
	})
	person, isErr := parseToolResult(t, resp)
	require.False(t, isErr, "crm_person_create with github_url should not return an error")
	assert.Equal(t, "Alice", person["first_name"])
	assert.Equal(t, "https://github.com/alice", person["github_url"])
}

// toolResultText reads a tool result's text + isError without JSON-decoding the
// content — for tools that return a plain confirmation (e.g. deletes) rather than
// a JSON entity, where parseToolResult's json.Unmarshal would fail.
func toolResultText(t *testing.T, resp gomcp.JSONRPCMessage) (string, bool) {
	t.Helper()
	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	var envelope struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(raw, &envelope))
	text := ""
	if len(envelope.Result.Content) > 0 {
		text = envelope.Result.Content[0].Text
	}
	return text, envelope.Result.IsError
}

// TestDeleteRemovesEntity verifies each new delete tool archives the record (so a
// subsequent get returns a not-found error) and returns a text confirmation.
func TestDeleteRemovesEntity(t *testing.T) {
	cases := []struct {
		name       string
		createTool string
		getTool    string
		deleteTool string
		createArgs map[string]any
	}{
		{"org", "crm_org_create", "crm_org_get", "crm_org_delete", map[string]any{"name": "Del Org"}},
		{"signal", "crm_signal_create", "crm_signal_get", "crm_signal_delete", map[string]any{"signal_type": "github"}},
		{"deal", "crm_deal_create", "crm_deal_get", "crm_deal_delete", map[string]any{"title": "Del Deal"}},
		{"task", "crm_task_create", "crm_task_get", "crm_task_delete", map[string]any{"title": "Del Task"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := setupTestServer(t)

			created, isErr := parseToolResult(t, callTool(t, s, tc.createTool, tc.createArgs))
			require.False(t, isErr, "%s create should not error", tc.name)
			id, ok := created["id"].(float64)
			require.True(t, ok, "%s id should be a number", tc.name)

			text, isErr := toolResultText(t, callTool(t, s, tc.deleteTool, map[string]any{"id": id}))
			require.False(t, isErr, "%s delete should not error", tc.name)
			assert.Contains(t, text, "deleted", "%s delete should confirm", tc.name)

			_, isErr = parseToolResult(t, callTool(t, s, tc.getTool, map[string]any{"id": id}))
			assert.True(t, isErr, "%s get after delete should be a not-found error", tc.name)
		})
	}
}

// TestTaskUpdate exercises crm_task_update patch semantics.
func TestTaskUpdate(t *testing.T) {
	s := setupTestServer(t)

	created, isErr := parseToolResult(t, callTool(t, s, "crm_task_create", map[string]any{"title": "Follow up"}))
	require.False(t, isErr, "crm_task_create should not error")
	id, ok := created["id"].(float64)
	require.True(t, ok, "task id should be a number")

	updated, isErr := parseToolResult(t, callTool(t, s, "crm_task_update", map[string]any{
		"id":       id,
		"priority": "high",
	}))
	require.False(t, isErr, "crm_task_update should not error")
	assert.Equal(t, "high", updated["priority"], "priority should be patched")
	assert.Equal(t, "Follow up", updated["title"], "untouched fields should be preserved")
}

// TestTaskCompleteThenConflict verifies crm_task_complete succeeds once and that a
// second call surfaces the repo's conflict error through the MCP layer.
func TestTaskCompleteThenConflict(t *testing.T) {
	s := setupTestServer(t)

	created, isErr := parseToolResult(t, callTool(t, s, "crm_task_create", map[string]any{"title": "Ship it"}))
	require.False(t, isErr, "crm_task_create should not error")
	id, ok := created["id"].(float64)
	require.True(t, ok, "task id should be a number")

	done, isErr := parseToolResult(t, callTool(t, s, "crm_task_complete", map[string]any{"id": id}))
	require.False(t, isErr, "first crm_task_complete should not error")
	assert.Equal(t, true, done["completed"], "task should be marked completed")

	_, isErr = parseToolResult(t, callTool(t, s, "crm_task_complete", map[string]any{"id": id}))
	assert.True(t, isErr, "completing an already-completed task should surface a conflict error")
}
