package operators

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"time"

	"github.com/nexssp/kernel/xerr"
	"github.com/nexssp/kernel/xfs"
)

// StateFileName is the on-disk name of the incremental cache. Written
// in the workspace root; overwritten on every successful run.
const StateFileName = ".nexss.srcpack.state"

// FileState is one file's last-seen hash and metadata. Keyed by
// RelPath in StateCache so the cache is portable across machines.
type FileState struct {
	Hash    string    `json:"hash"`
	ModTime time.Time `json:"mod_time"`
	Size    int64     `json:"size"`
}

// StateCache is the on-disk form of the incremental cache.
type StateCache struct {
	LastRun time.Time            `json:"last_run"`
	Files   map[string]FileState `json:"files"`
}

// LoadState reads the cache from path. A missing or unreadable file
// returns (nil, nil) — the cache is best-effort, never authoritative.
// A corrupt file is also treated as absent.
func LoadState(path string) *StateCache {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cache StateCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil
	}
	if cache.Files == nil {
		cache.Files = map[string]FileState{}
	}
	return &cache
}

// SaveState writes the current set of files as the new state cache.
// Omitted files are skipped; a nil cache target path is a no-op.
func SaveState(path string, files []ParsedFile) error {
	cache := StateCache{
		LastRun: time.Now().UTC(),
		Files:   make(map[string]FileState, len(files)),
	}
	for i := range files {
		file := &files[i]
		if file.Omitted {
			continue
		}
		cache.Files[file.RelPath] = FileState{
			Hash:    file.Hash,
			ModTime: file.ModTime,
			Size:    file.Size,
		}
	}
	data, err := json.MarshalIndent(&cache, "", "  ")
	if err != nil {
		return xerr.Internal("marshal state cache", err)
	}
	if err := xfs.WriteFileAtomic(path, data, 0o600); err != nil {
		return xerr.Internal("write state cache "+path, err)
	}
	return nil
}

// IsChanged reports whether relPath needs reprocessing. A file not
// present in the cache is always considered changed. An unchanged
// ModTime short-circuits the hash comparison.
func (s *StateCache) IsChanged(relPath, absPath string, modTime time.Time) (bool, error) {
	if s == nil {
		return true, nil
	}
	recorded, ok := s.Files[relPath]
	if !ok {
		return true, nil
	}
	if !modTime.IsZero() && !recorded.ModTime.IsZero() && modTime.Equal(recorded.ModTime) {
		return false, nil
	}
	hash, err := hashFile(absPath)
	if err != nil {
		return false, err
	}
	return hash != recorded.Hash, nil
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
