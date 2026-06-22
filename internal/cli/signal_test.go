package cli_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignalAdd(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")

	stdout, _, code := crm(t, dbPath, "signal", "add", "github", "--description", "New RL repo", "-f", "json")
	assert.Equal(t, 0, code)

	var data []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &data))
	assert.Equal(t, "github", data[0]["signal_type"])
	assert.Equal(t, "New RL repo", data[0]["description"])
	assert.NotEmpty(t, data[0]["detected_at"])
}

func TestSignalList(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "signal", "add", "github", "-f", "json")
	crm(t, dbPath, "signal", "add", "arxiv", "-f", "json")

	stdout, _, code := crm(t, dbPath, "signal", "list", "-f", "json")
	assert.Equal(t, 0, code)

	var data []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &data))
	assert.Len(t, data, 2)
}

func TestSignalListFilterByType(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "signal", "add", "github", "-f", "json")
	crm(t, dbPath, "signal", "add", "arxiv", "-f", "json")

	stdout, _, code := crm(t, dbPath, "signal", "list", "--type", "arxiv", "-f", "json")
	assert.Equal(t, 0, code)

	var data []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &data))
	assert.Len(t, data, 1)
	assert.Equal(t, "arxiv", data[0]["signal_type"])
}

func TestSignalShow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "signal", "add", "funding", "-f", "json")

	stdout, _, code := crm(t, dbPath, "signal", "show", "1", "-f", "json")
	assert.Equal(t, 0, code)

	var data []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &data))
	assert.Equal(t, "funding", data[0]["signal_type"])
}

func TestSignalDelete(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "signal", "add", "github", "-f", "json")

	_, stderr, code := crm(t, dbPath, "signal", "delete", "1")
	assert.Equal(t, 0, code)
	assert.Contains(t, stderr, "Deleted signal #1")

	_, _, code = crm(t, dbPath, "signal", "show", "1")
	assert.Equal(t, 3, code) // not found
}
