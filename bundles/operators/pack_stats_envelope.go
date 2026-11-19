package operators

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

type packStatsEnvelope struct {
	Files []ParsedFile
	Stats PackStats
}

func isPackStatsEnvelope(value any) bool {
	switch value.(type) {
	case packStatsEnvelope, *packStatsEnvelope:
		return true
	case map[string]any:
		_, ok := projectedRecordValue(value.(map[string]any), "files", "Files")
		return ok
	default:
		return false
	}
}

func unpackPackStatsEnvelope(value any) (packStatsEnvelope, error) {
	switch envelope := value.(type) {
	case packStatsEnvelope:
		return envelope, nil
	case *packStatsEnvelope:
		if envelope == nil {
			return packStatsEnvelope{}, errors.New("NFlow pack fold returned a nil envelope")
		}
		return *envelope, nil
	}

	fields, ok := value.(map[string]any)
	if !ok {
		return packStatsEnvelope{}, fmt.Errorf("NFlow pack fold returned %T, want map[string]any", value)
	}

	rawFiles, ok := projectedRecordValue(fields, "files", "Files")
	if !ok {
		return packStatsEnvelope{}, errors.New("NFlow pack fold result has no files field")
	}
	files, err := parsedFilesFromEnvelope(rawFiles)
	if err != nil {
		return packStatsEnvelope{}, err
	}

	statsValue, _ := projectedRecordValue(fields, "stats", "Stats")
	statsFields, _ := statsValue.(map[string]any)
	statValue := func(keys ...string) (any, bool) {
		if value, exists := projectedRecordValue(fields, keys...); exists {
			return value, true
		}
		return projectedRecordValue(statsFields, keys...)
	}
	fileCountValue, _ := statValue("fileCount", "FileCount", "file_count")
	fileCount, err := foldInteger(fileCountValue)
	if err != nil {
		return packStatsEnvelope{}, fmt.Errorf("NFlow pack fold fileCount: %w", err)
	}
	funcCountValue, _ := statValue("funcCount", "FuncCount", "func_count")
	funcCount, err := foldInteger(funcCountValue)
	if err != nil {
		return packStatsEnvelope{}, fmt.Errorf("NFlow pack fold funcCount: %w", err)
	}
	packedBytesValue, _ := statValue("packedBytes", "PackedBytes", "packed_bytes")
	packedBytes, err := foldInteger(packedBytesValue)
	if err != nil {
		return packStatsEnvelope{}, fmt.Errorf("NFlow pack fold packedBytes: %w", err)
	}
	rawBytesValue, _ := statValue("rawBytes", "RawBytes", "raw_bytes")
	rawBytes, err := foldInteger(rawBytesValue)
	if err != nil {
		return packStatsEnvelope{}, fmt.Errorf("NFlow pack fold rawBytes: %w", err)
	}
	estimatedTokens := int64(float64(packedBytes) / 3.5)
	if value, ok := statValue("estimatedTokens", "EstimatedTokens", "estimated_tokens"); ok {
		estimatedTokens, err = foldInteger(value)
		if err != nil {
			return packStatsEnvelope{}, fmt.Errorf("NFlow pack fold estimatedTokens: %w", err)
		}
	}

	stats := PackStats{
		FileCount:       int(fileCount),
		FuncCount:       int(funcCount),
		PackedBytes:     packedBytes,
		RawBytes:        rawBytes,
		EstimatedTokens: estimatedTokens,
	}

	return packStatsEnvelope{Files: files, Stats: stats}, nil
}

func parsedFilesFromEnvelope(value any) ([]ParsedFile, error) {
	var rawFiles []any
	switch files := value.(type) {
	case []any:
		rawFiles = files
	case []ParsedFile:
		return append([]ParsedFile(nil), files...), nil
	default:
		return nil, fmt.Errorf("NFlow pack fold files has type %T, want an array of ParsedFile records", value)
	}

	files := make([]ParsedFile, 0, len(rawFiles))
	for i, raw := range rawFiles {
		file, err := parsedFileFromProjectedRecord(raw)
		if err != nil {
			return nil, fmt.Errorf("NFlow pack fold files[%d]: %w", i, err)
		}
		files = append(files, file)
	}
	return files, nil
}

func parsedFilesFromRecords(records []any) ([]ParsedFile, error) {
	files := make([]ParsedFile, 0, len(records))
	for i, record := range records {
		file, err := parsedFileFromProjectedRecord(record)
		if err != nil {
			return nil, fmt.Errorf("parsed-file stream item %d: %w", i, err)
		}
		files = append(files, file)
	}
	return files, nil
}

func parsedFileFromProjectedRecord(value any) (ParsedFile, error) {
	if file, ok := value.(ParsedFile); ok {
		return file, nil
	}
	record, ok := value.(map[string]any)
	if !ok {
		return ParsedFile{}, fmt.Errorf("record has type %T, want operators.ParsedFile or a projected record map", value)
	}
	contentValue, ok := projectedRecordValue(record, "Content", "content")
	if !ok {
		return ParsedFile{}, errors.New("projected record has no content field")
	}
	content, err := projectedContentBytes(contentValue)
	if err != nil {
		return ParsedFile{}, err
	}

	// Flow projections expose Go fields and JSON-tag names, and turn []byte into
	// []any of byte values. Rebuild only the serializable ParsedFile fields used
	// downstream; the embedded time.Time method map is intentionally excluded.
	fields := map[string]any{"Content": content}
	for _, field := range []struct {
		name string
		keys []string
	}{
		{"path", []string{"path", "Path"}},
		{"rel_path", []string{"rel_path", "RelPath"}},
		{"size", []string{"size", "Size"}},
		{"lang", []string{"lang", "Lang"}},
		{"Snippets", []string{"Snippets", "snippets"}},
		{"FuncCount", []string{"FuncCount", "func_count"}},
		{"Lines", []string{"Lines", "lines"}},
		{"source_start_line", []string{"source_start_line", "SourceStartLine"}},
		{"source_end_line", []string{"source_end_line", "SourceEndLine"}},
		{"sliced", []string{"sliced", "Sliced"}},
		{"source_url", []string{"source_url", "SourceURL"}},
		{"symbol_spans", []string{"symbol_spans", "SymbolSpans"}},
		{"line_spans", []string{"line_spans", "LineSpans"}},
		{"IsRaw", []string{"IsRaw", "is_raw"}},
		{"IsFull", []string{"IsFull", "is_full"}},
		{"Omitted", []string{"Omitted", "omitted"}},
		{"OmitReason", []string{"OmitReason", "omit_reason"}},
		{"ExpandTree", []string{"ExpandTree", "expand_tree"}},
		{"Hash", []string{"Hash", "hash"}},
		{"AbsPath", []string{"AbsPath", "abs_path"}},
		{"Profile", []string{"Profile", "profile"}},
		{"Prompt", []string{"Prompt", "prompt"}},
		{"ParseOptions", []string{"ParseOptions", "parse_options"}},
		{"ParseOptionsSet", []string{"ParseOptionsSet", "parse_options_set"}},
	} {
		if fieldValue, exists := projectedRecordValue(record, field.keys...); exists {
			fields[field.name] = fieldValue
		}
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return ParsedFile{}, fmt.Errorf("encode projected record: %w", err)
	}
	var file ParsedFile
	if err := json.Unmarshal(data, &file); err != nil {
		return ParsedFile{}, fmt.Errorf("decode projected record: %w", err)
	}
	return file, nil
}

func projectedRecordValue(record map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, ok := record[key]; ok {
			return value, true
		}
	}
	return nil, false
}

func projectedContentBytes(value any) ([]byte, error) {
	switch content := value.(type) {
	case []byte:
		return append([]byte(nil), content...), nil
	case string:
		return []byte(content), nil
	case []any:
		data := make([]byte, len(content))
		for i, item := range content {
			var number float64
			switch n := item.(type) {
			case uint8:
				number = float64(n)
			case int:
				number = float64(n)
			case int64:
				number = float64(n)
			case uint64:
				number = float64(n)
			case float64:
				number = n
			default:
				return nil, fmt.Errorf("projected content byte %d has type %T", i, item)
			}
			if math.IsNaN(number) || math.IsInf(number, 0) || number < 0 || number > 255 || math.Trunc(number) != number {
				return nil, fmt.Errorf("projected content byte %d is outside the byte range: %v", i, number)
			}
			data[i] = byte(number)
		}
		return data, nil
	default:
		return nil, fmt.Errorf("projected content has type %T, want bytes", value)
	}
}
