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

func TestSignalAddEmptyAtUsesDefault(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")

	stdout, _, code := crm(t, dbPath, "signal", "add", "github", "--at", "", "-f", "json")
	assert.Equal(t, 0, code)

	var data []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &data))
	// An empty --at must fall through to the NOT NULL default, not store "".
	assert.NotEmpty(t, data[0]["detected_at"])
}

func TestSignalAddWithDetectedAt(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")

	stdout, _, code := crm(t, dbPath, "signal", "add", "funding", "--at", "2026-01-15 09:00:00", "-f", "json")
	assert.Equal(t, 0, code)

	var data []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &data))
	assert.Equal(t, "2026-01-15 09:00:00", data[0]["detected_at"])
}

func TestSignalAddSourceURLDedups(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	url := "https://github.com/acme/repo"

	out1, _, code1 := crm(t, dbPath, "signal", "add", "github", "--source-url", url, "-f", "json")
	assert.Equal(t, 0, code1)
	out2, _, code2 := crm(t, dbPath, "signal", "add", "github", "--source-url", url, "-f", "json")
	assert.Equal(t, 0, code2)

	var a, b []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out1), &a))
	require.NoError(t, json.Unmarshal([]byte(out2), &b))
	assert.Equal(t, url, a[0]["source_url"])
	assert.Equal(t, a[0]["id"], b[0]["id"], "same source_url returns the same signal id")

	list, _, _ := crm(t, dbPath, "signal", "list", "-f", "json")
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(list), &rows))
	assert.Len(t, rows, 1)
}

func TestSignalEditLinksOrg(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "org", "add", "Acme AI", "-f", "json")                                                // org id 1
	crm(t, dbPath, "signal", "add", "github", "--source-url", "https://github.com/acme/x", "-f", "json") // signal id 1, org-less

	stdout, _, code := crm(t, dbPath, "signal", "edit", "1", "--org", "1", "-f", "json")
	assert.Equal(t, 0, code)

	var data []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &data))
	assert.Equal(t, float64(1), data[0]["org_id"], "signal should now be linked to org 1")
}
