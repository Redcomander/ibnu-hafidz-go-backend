package diagnostics

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPersistenceResolutionAndDebugExpiry(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	require.NoError(t, err)
	event := Event{ID: NewID(), RequestID: NewID(), CreatedAt: time.Now().UTC(), Status: 500}
	require.NoError(t, store.Add(event))
	require.NoError(t, store.SetDebug(true))
	require.True(t, store.DebugEnabled())
	require.NoError(t, store.Resolve(event.ID, true, 7))
	restored, err := New(dir)
	require.NoError(t, err)
	records := restored.Snapshot()
	require.Len(t, records, 1)
	require.True(t, records[0].Resolved)
	require.EqualValues(t, 7, records[0].ResolvedBy)
	require.True(t, restored.DebugEnabled())
	require.NoError(t, restored.Resolve(event.ID, false, 7))
	require.Nil(t, restored.Snapshot()[0].ResolvedAt)
	require.ErrorIs(t, restored.Resolve("../../private", true, 1), os.ErrNotExist)
	restored.debugUntil = time.Now().Add(-time.Second)
	require.False(t, restored.DebugEnabled())
	require.NoError(t, restored.SetDebug(false))
	require.False(t, restored.DebugEnabled())
}

func TestRetentionOnlyRemovesOwnedDiagnostics(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("existing file"), 0600))
	store, err := New(dir)
	require.NoError(t, err)
	old := Event{ID: NewID(), CreatedAt: time.Now().Add(-8 * 24 * time.Hour)}
	require.NoError(t, store.Add(old))
	require.Empty(t, store.Snapshot())
	_, err = os.Stat(filepath.Join(dir, "event-"+old.ID+".json"))
	require.True(t, os.IsNotExist(err))
	require.FileExists(t, filepath.Join(dir, "keep.txt"))
	memory := Memory()
	for i := 0; i < MaxEvents+3; i++ {
		_ = memory.Add(Event{ID: NewID(), CreatedAt: time.Now()})
	}
	require.Len(t, memory.Snapshot(), MaxEvents)
}

func TestFailedSaveDoesNotPretendDebugOrResolutionSucceeded(t *testing.T) {
	memory := Memory()
	event := Event{ID: NewID(), CreatedAt: time.Now()}
	require.Error(t, memory.Add(event))
	require.Error(t, memory.SetDebug(true))
	require.False(t, memory.DebugEnabled())
	require.Error(t, memory.Resolve(event.ID, true, 3))
	require.False(t, memory.Snapshot()[0].Resolved)
	require.Equal(t, false, memory.Settings()["storage_healthy"])
}
