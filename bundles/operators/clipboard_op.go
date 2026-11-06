package operators

import (
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/atotto/clipboard"
	"github.com/nexssp/flow/extensions/fs"
	"github.com/nexssp/kernel/action"
)

type ClipboardConfig struct {
	Enabled bool `json:"enabled"`
	Quiet   bool `json:"quiet"`
}

// UnmarshalJSON accepts Flow v0.10 modifier values, which arrive as strings,
// as well as boolean values from direct Go callers.
func (cfg *ClipboardConfig) UnmarshalJSON(data []byte) error {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	if raw, ok := values["enabled"]; ok && string(raw) != "null" {
		if len(raw) > 0 && raw[0] == '"' {
			var text string
			if err := json.Unmarshal(raw, &text); err != nil {
				return err
			}
			enabled, err := strconv.ParseBool(text)
			if err != nil {
				return fmt.Errorf("enabled must be true or false, got %q", text)
			}
			cfg.Enabled = enabled
		} else if err := json.Unmarshal(raw, &cfg.Enabled); err != nil {
			return err
		}
	}
	if raw, ok := values["quiet"]; ok && string(raw) != "null" {
		if len(raw) > 0 && raw[0] == '"' {
			var text string
			if err := json.Unmarshal(raw, &text); err != nil {
				return err
			}
			quiet, err := strconv.ParseBool(text)
			if err != nil {
				return fmt.Errorf("quiet must be true or false, got %q", text)
			}
			cfg.Quiet = quiet
		} else if err := json.Unmarshal(raw, &cfg.Quiet); err != nil {
			return err
		}
	}
	return nil
}

type clipboardSanitization struct {
	nulBytesRemoved     int
	invalidUTF8Replaced bool
}

// sanitizeClipboardText prevents Windows' syscall.StringToUTF16 from panicking
// on embedded NULs. It is applied only to the clipboard copy: the pipeline's
// original rendered bytes are passed downstream unchanged.
func sanitizeClipboardText(text string) (string, clipboardSanitization) {
	result := clipboardSanitization{nulBytesRemoved: strings.Count(text, "\x00")}
	if result.nulBytesRemoved > 0 {
		text = strings.ReplaceAll(text, "\x00", "")
	}
	if !utf8.ValidString(text) {
		result.invalidUTF8Replaced = true
		text = strings.ToValidUTF8(text, "\uFFFD")
	}
	return text, result
}

func writeSystemClipboard(text string) error {
	return clipboard.WriteAll(text)
}

// Clipboard copies the rendered stream to the system clipboard when enabled,
// then forwards the original items unchanged.
func Clipboard(cfg ClipboardConfig) action.StreamOp[fs.FileContent, fs.FileContent] {
	return clipboardWithWriter(cfg, writeSystemClipboard)
}

func clipboardWithWriter(cfg ClipboardConfig, writeClipboard func(string) error) action.StreamOp[fs.FileContent, fs.FileContent] {
	return func(up iter.Seq2[fs.FileContent, error]) iter.Seq2[fs.FileContent, error] {
		return func(yield func(fs.FileContent, error) bool) {
			if !cfg.Enabled {
				for item, err := range up {
					if !yield(item, err) {
						return
					}
				}
				return
			}

			items := make([]fs.FileContent, 0)
			var contents strings.Builder
			for item, err := range up {
				if err != nil {
					yield(fs.FileContent{}, err)
					return
				}
				items = append(items, item)
				contents.Write(item.Content)
			}
			if len(items) == 0 {
				if !cfg.Quiet {
					fmt.Fprintln(os.Stderr, "Clipboard: skipped (no rendered content)")
				}
				return
			}

			clipboardText, sanitized := sanitizeClipboardText(contents.String())
			if err := writeClipboard(clipboardText); err != nil {
				yield(fs.FileContent{}, fmt.Errorf("copy rendered output to clipboard: %w", err))
				return
			}
			if !cfg.Quiet {
				if sanitized.nulBytesRemoved > 0 || sanitized.invalidUTF8Replaced {
					details := make([]string, 0, 2)
					if sanitized.nulBytesRemoved > 0 {
						details = append(details, fmt.Sprintf("removed %d embedded NUL byte(s)", sanitized.nulBytesRemoved))
					}
					if sanitized.invalidUTF8Replaced {
						details = append(details, "replaced invalid UTF-8 with U+FFFD")
					}
					fmt.Fprintf(os.Stderr, "Clipboard note: %s; saved/rendered context is unchanged.\n", strings.Join(details, " and "))
				}
				fmt.Fprintln(os.Stderr, "Clipboard: copied rendered context")
			}

			for _, item := range items {
				if !yield(item, nil) {
					return
				}
			}
		}
	}
}

func ClipboardOperator() action.NamedOperator {
	return clipboardOperatorWithWriter(writeSystemClipboard)
}

func clipboardOperatorWithWriter(writeClipboard func(string) error) action.NamedOperator {
	return action.NewOperator("clipboard.write", func(cfg ClipboardConfig) action.StreamOp[fs.FileContent, fs.FileContent] {
		return clipboardWithWriter(cfg, writeClipboard)
	})
}
