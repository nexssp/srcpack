package operators

import (
	"encoding/json"
	"fmt"
	"iter"
	"log/slog"
	"strconv"

	"github.com/nexssp/kernel/action"
)

type SaveStateConfig struct {
	Enabled *bool `json:"enabled"`
}

// UnmarshalJSON accepts Flow modifier boolean strings as well as JSON booleans.
func (cfg *SaveStateConfig) UnmarshalJSON(data []byte) error {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	if raw, ok := values["enabled"]; ok && string(raw) != "null" {
		var enabled bool
		if len(raw) > 0 && raw[0] == '"' {
			var text string
			if err := json.Unmarshal(raw, &text); err != nil {
				return err
			}
			parsed, err := strconv.ParseBool(text)
			if err != nil {
				return fmt.Errorf("enabled must be true or false, got %q", text)
			}
			enabled = parsed
		} else if err := json.Unmarshal(raw, &enabled); err != nil {
			return err
		}
		cfg.Enabled = &enabled
	}
	return nil
}

func (cfg SaveStateConfig) isEnabled() bool {
	return cfg.Enabled == nil || *cfg.Enabled
}

func SaveStateOperator() action.NamedOperator {
	return action.NewOperator("workspace.save_state", saveState)
}

func saveState(cfg SaveStateConfig) action.StreamOp[ParsedFile, ParsedFile] {
	return func(up iter.Seq2[ParsedFile, error]) iter.Seq2[ParsedFile, error] {
		return func(yield func(ParsedFile, error) bool) {
			if !cfg.isEnabled() {
				for file, err := range up {
					if !yield(file, err) {
						return
					}
				}
				return
			}
			var files []ParsedFile
			for file, err := range up {
				if err != nil {
					yield(ParsedFile{}, err)
					return
				}
				files = append(files, file)
			}
			// Best-effort: a state write failure must not abort the pack.
			if saveErr := SaveState(StateFileName, files); saveErr != nil {
				slog.Warn("srcpack: state cache write failed", "error", saveErr)
			}
			for i := range files {
				if !yield(files[i], nil) {
					return
				}
			}
		}
	}
}
