package operators

import (
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/nexssp/kernel/xerr"
)

const (
	defaultMaxFileSize  int64 = 10 << 20 // 10 MiB
	defaultMaxFilesScan int64 = 200_000
)

// Guardrails enforces per-scan safety limits. A non-positive limit is
// treated as "use the default" rather than "unlimited" — an unbounded
// scan is never the caller's intent.
type Guardrails struct {
	MaxFileSize  int64
	MaxFilesScan int64

	scanned atomic.Int64
}

// NewGuardrails returns a Guardrails with defaults applied for any
// non-positive limit.
func NewGuardrails(maxFileSize, maxFilesScan int64) *Guardrails {
	if maxFileSize <= 0 {
		maxFileSize = defaultMaxFileSize
	}
	if maxFilesScan <= 0 {
		maxFilesScan = defaultMaxFilesScan
	}
	return &Guardrails{MaxFileSize: maxFileSize, MaxFilesScan: maxFilesScan}
}

// CheckFile counts the file against MaxFilesScan and rejects files
// larger than MaxFileSize with xerr.Forbidden. A nil receiver is a
// no-op so callers can disable the limits.
func (g *Guardrails) CheckFile(path string, info os.FileInfo) error {
	if g == nil {
		return nil
	}
	if g.scanned.Add(1) > g.MaxFilesScan {
		return xerr.Forbidden("scan runaway prevented: exceeded maximum scan limit")
	}
	if info != nil && info.Size() > g.MaxFileSize {
		return xerr.Forbidden("file " + filepath.Base(path) + " exceeds maximum allowed size")
	}
	return nil
}
