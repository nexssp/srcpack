package operators

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/nexssp/kernel/action"
)

const (
	documentFieldContent     = "content"
	documentFieldPackedBytes = "packed_bytes"
	documentFieldRawBytes    = "raw_bytes"
)

// DocumentInputAction exposes typed domain facts. Placeholder names, aliases,
// defaults and the document bundle's file shape belong to render.nflow.
func DocumentInputAction() action.AnyAction {
	return action.New("srcpack.document_input", func(ctx context.Context, input map[string]any) (map[string]any, error) {
		records, ok := input["records"].([]any)
		if !ok {
			return nil, errors.New("srcpack.document_input: records must be an array")
		}
		var cfg RenderConfig
		normalized, normalizeErr := action.NormalizeConfigForTarget(input["config"], &cfg)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		encoded, marshalErr := json.Marshal(normalized)
		if marshalErr != nil {
			return nil, marshalErr
		}
		if unmarshalErr := json.Unmarshal(encoded, &cfg); unmarshalErr != nil {
			return nil, unmarshalErr
		}
		files, stats, err := collectRenderRecords(records)
		if err != nil {
			return nil, err
		}
		return documentInput(ctx, cfg, files, stats)
	}).Description("Expose typed source facts and lossless content for NFlow document shaping").Build()
}

func documentInput(ctx context.Context, cfg RenderConfig, files []ParsedFile, stats PackStats) (map[string]any, error) {
	tree, err := renderConfiguredTreeContext(ctx, cfg, files)
	if err != nil {
		return nil, err
	}
	// Configuration alone crosses JSON. Source bytes never do: Go's direct
	// []byte-to-string conversion preserves NUL and invalid UTF-8 bit-for-bit.
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var config map[string]any
	if err := json.Unmarshal(encoded, &config); err != nil {
		return nil, err
	}
	facts := make([]map[string]any, 0, len(files))
	for i := range files {
		file := &files[i]
		facts = append(facts, map[string]any{
			documentFieldContent: string(file.Content), "snippets": file.Snippets, "omitted": file.Omitted,
			"rel_path": file.RelPath, "abs_path": file.Path, "lang": file.Lang,
			"lines": file.Lines, "size": file.Size,
			"source_start_line": file.SourceStartLine, "source_end_line": file.SourceEndLine,
			"source_url":   file.SourceURL,
			"symbol_spans": file.SymbolSpans, "source_location": string(sourceLocationComment(file)),
		})
	}
	return map[string]any{
		"config": config, "tree": tree, "files": facts,
		"stats": map[string]any{
			"file_count": stats.FileCount, "func_count": stats.FuncCount,
			documentFieldPackedBytes: stats.PackedBytes, documentFieldRawBytes: stats.RawBytes,
			"estimated_tokens": stats.EstimatedTokens,
		},
	}, nil
}
