package repo_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Array-Ventures/gtm-crm/internal/db"
	"github.com/Array-Ventures/gtm-crm/internal/db/repo"
	"github.com/Array-Ventures/gtm-crm/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSignalTestDB(t *testing.T) (*repo.SignalRepo, *repo.PersonRepo, *repo.OrgRepo) {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return repo.NewSignalRepo(d), repo.NewPersonRepo(d), repo.NewOrgRepo(d)
}

func TestSignalCreate(t *testing.T) {
	sr, _, _ := setupSignalTestDB(t)
	ctx := context.Background()

	signal, err := sr.Create(ctx, model.CreateSignalInput{SignalType: "github"})
	require.NoError(t, err)
	assert.Equal(t, "github", signal.SignalType)
	assert.NotEmpty(t, signal.UUID)
	assert.NotEmpty(t, signal.DetectedAt)
}

func TestSignalCreate_WithLinksAndDescription(t *testing.T) {
	sr, pr, or := setupSignalTestDB(t)
	ctx := context.Background()

	p, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane"})
	require.NoError(t, err)
	o, err := or.Create(ctx, model.CreateOrgInput{Name: "Acme AI"})
	require.NoError(t, err)
	desc := "Published a new RLHF paper"

	signal, err := sr.Create(ctx, model.CreateSignalInput{
		SignalType:  "arxiv",
		Description: &desc,
		PersonID:    &p.ID,
		OrgID:       &o.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, &desc, signal.Description)
	assert.Equal(t, &p.ID, signal.PersonID)
	assert.Equal(t, &o.ID, signal.OrgID)
}

func TestSignalCreate_EmptyTypeInvalid(t *testing.T) {
	sr, _, _ := setupSignalTestDB(t)
	_, err := sr.Create(context.Background(), model.CreateSignalInput{SignalType: ""})
	assert.ErrorIs(t, err, model.ErrValidation)
}

func TestSignalCreate_CustomDetectedAt(t *testing.T) {
	sr, _, _ := setupSignalTestDB(t)
	at := "2026-01-15 09:00:00"
	signal, err := sr.Create(context.Background(), model.CreateSignalInput{
		SignalType: "funding",
		DetectedAt: &at,
	})
	require.NoError(t, err)
	assert.Equal(t, at, signal.DetectedAt)
}

func TestSignalFindByID(t *testing.T) {
	sr, _, _ := setupSignalTestDB(t)
	ctx := context.Background()

	created, err := sr.Create(ctx, model.CreateSignalInput{SignalType: "jobs"})
	require.NoError(t, err)

	found, err := sr.FindByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
}

func TestSignalFindByID_NotFound(t *testing.T) {
	sr, _, _ := setupSignalTestDB(t)
	_, err := sr.FindByID(context.Background(), 999)
	assert.ErrorIs(t, err, model.ErrNotFound)
}

func TestSignalFindAll(t *testing.T) {
	sr, _, _ := setupSignalTestDB(t)
	ctx := context.Background()

	_, err := sr.Create(ctx, model.CreateSignalInput{SignalType: "github"})
	require.NoError(t, err)
	_, err = sr.Create(ctx, model.CreateSignalInput{SignalType: "arxiv"})
	require.NoError(t, err)

	signals, err := sr.FindAll(ctx, model.SignalFilters{})
	require.NoError(t, err)
	assert.Len(t, signals, 2)
}

func TestSignalFindAll_FilterByType(t *testing.T) {
	sr, _, _ := setupSignalTestDB(t)
	ctx := context.Background()

	_, err := sr.Create(ctx, model.CreateSignalInput{SignalType: "github"})
	require.NoError(t, err)
	_, err = sr.Create(ctx, model.CreateSignalInput{SignalType: "arxiv"})
	require.NoError(t, err)

	st := "arxiv"
	signals, err := sr.FindAll(ctx, model.SignalFilters{SignalType: &st})
	require.NoError(t, err)
	assert.Len(t, signals, 1)
	assert.Equal(t, "arxiv", signals[0].SignalType)
}

func TestSignalFindAll_FilterByOrg(t *testing.T) {
	sr, _, or := setupSignalTestDB(t)
	ctx := context.Background()

	o, err := or.Create(ctx, model.CreateOrgInput{Name: "Acme AI"})
	require.NoError(t, err)
	_, err = sr.Create(ctx, model.CreateSignalInput{SignalType: "github", OrgID: &o.ID})
	require.NoError(t, err)
	_, err = sr.Create(ctx, model.CreateSignalInput{SignalType: "github"})
	require.NoError(t, err)

	signals, err := sr.FindAll(ctx, model.SignalFilters{OrgID: &o.ID})
	require.NoError(t, err)
	assert.Len(t, signals, 1)
	assert.Equal(t, &o.ID, signals[0].OrgID)
}

func TestSignalUpdate(t *testing.T) {
	sr, _, _ := setupSignalTestDB(t)
	ctx := context.Background()

	signal, err := sr.Create(ctx, model.CreateSignalInput{SignalType: "github"})
	require.NoError(t, err)

	newType := "arxiv"
	desc := "updated"
	updated, err := sr.Update(ctx, signal.ID, model.UpdateSignalInput{
		SignalType:  &newType,
		Description: &desc,
	})
	require.NoError(t, err)
	assert.Equal(t, "arxiv", updated.SignalType)
	assert.Equal(t, &desc, updated.Description)
}

func TestSignalUpdate_EmptyTypeInvalid(t *testing.T) {
	sr, _, _ := setupSignalTestDB(t)
	ctx := context.Background()

	signal, err := sr.Create(ctx, model.CreateSignalInput{SignalType: "github"})
	require.NoError(t, err)

	empty := ""
	_, err = sr.Update(ctx, signal.ID, model.UpdateSignalInput{SignalType: &empty})
	assert.ErrorIs(t, err, model.ErrValidation)
}

func TestSignalArchive(t *testing.T) {
	sr, _, _ := setupSignalTestDB(t)
	ctx := context.Background()

	signal, err := sr.Create(ctx, model.CreateSignalInput{SignalType: "github"})
	require.NoError(t, err)

	err = sr.Archive(ctx, signal.ID)
	require.NoError(t, err)

	_, err = sr.FindByID(ctx, signal.ID)
	assert.ErrorIs(t, err, model.ErrNotFound)
}

func TestSignalCreate_StoresSourceURL(t *testing.T) {
	sr, _, _ := setupSignalTestDB(t)
	url := "https://github.com/collinear-ai/verl-trainer"

	created, err := sr.Create(context.Background(), model.CreateSignalInput{
		SignalType: "github",
		SourceURL:  &url,
	})
	require.NoError(t, err)
	require.NotNil(t, created.SourceURL)
	assert.Equal(t, url, *created.SourceURL)

	found, err := sr.FindByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.NotNil(t, found.SourceURL)
	assert.Equal(t, url, *found.SourceURL)
}

func TestSignalCreate_DedupsOnSourceURL(t *testing.T) {
	sr, _, _ := setupSignalTestDB(t)
	ctx := context.Background()
	url := "https://github.com/acme/repo"

	first, err := sr.Create(ctx, model.CreateSignalInput{SignalType: "github", SourceURL: &url})
	require.NoError(t, err)
	second, err := sr.Create(ctx, model.CreateSignalInput{SignalType: "github", SourceURL: &url})
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID, "same source_url must return the same signal")

	all, err := sr.FindAll(ctx, model.SignalFilters{})
	require.NoError(t, err)
	assert.Len(t, all, 1, "no duplicate row")
}

func TestSignalCreate_DistinctSourceURLsCoexist(t *testing.T) {
	sr, _, _ := setupSignalTestDB(t)
	ctx := context.Background()
	a, b := "https://github.com/acme/a", "https://github.com/acme/b"

	_, err := sr.Create(ctx, model.CreateSignalInput{SignalType: "github", SourceURL: &a})
	require.NoError(t, err)
	_, err = sr.Create(ctx, model.CreateSignalInput{SignalType: "github", SourceURL: &b})
	require.NoError(t, err)

	all, err := sr.FindAll(ctx, model.SignalFilters{})
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestSignalCreate_NullSourceURLsDoNotCollide(t *testing.T) {
	sr, _, _ := setupSignalTestDB(t)
	ctx := context.Background()

	_, err := sr.Create(ctx, model.CreateSignalInput{SignalType: "github"})
	require.NoError(t, err)
	_, err = sr.Create(ctx, model.CreateSignalInput{SignalType: "github"})
	require.NoError(t, err)

	all, err := sr.FindAll(ctx, model.SignalFilters{})
	require.NoError(t, err)
	assert.Len(t, all, 2, "NULL source_url never collides")
}

func TestSignalCreate_ArchivedDoesNotBlockReinsert(t *testing.T) {
	sr, _, _ := setupSignalTestDB(t)
	ctx := context.Background()
	url := "https://github.com/acme/repo"

	first, err := sr.Create(ctx, model.CreateSignalInput{SignalType: "github", SourceURL: &url})
	require.NoError(t, err)
	require.NoError(t, sr.Archive(ctx, first.ID))

	second, err := sr.Create(ctx, model.CreateSignalInput{SignalType: "github", SourceURL: &url})
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, second.ID, "archived row is excluded by the partial index")

	all, err := sr.FindAll(ctx, model.SignalFilters{})
	require.NoError(t, err)
	assert.Len(t, all, 1, "only the live row is listed")
}

func TestSignalCreate_DedupAcrossConnections(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	d1, err := db.Open(dbPath)
	require.NoError(t, err)
	defer d1.Close()
	d2, err := db.Open(dbPath)
	require.NoError(t, err)
	defer d2.Close()

	ctx := context.Background()
	url := "https://github.com/acme/repo"
	first, err := repo.NewSignalRepo(d1).Create(ctx, model.CreateSignalInput{SignalType: "github", SourceURL: &url})
	require.NoError(t, err)
	second, err := repo.NewSignalRepo(d2).Create(ctx, model.CreateSignalInput{SignalType: "github", SourceURL: &url})
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID, "DB-enforced dedup holds across separate connections")
}
