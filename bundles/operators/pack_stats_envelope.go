package operators

import (
	"errors"
	"fmt"
)

type packStatsEnvelope struct {
	Files []ParsedFile
	Stats PackStats
}

func isPackStatsEnvelope(value any) bool {
	switch value.(type) {
	case map[string]any, packStatsEnvelope, *packStatsEnvelope:
		return true
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

	rawFiles, ok := fields["files"]
	if !ok {
		return packStatsEnvelope{}, errors.New("NFlow pack fold result has no files field")
	}
	files, err := parsedFilesFromEnvelope(rawFiles)
	if err != nil {
		return packStatsEnvelope{}, err
	}

	fileCount, err := foldInteger(fields["fileCount"])
	if err != nil {
		return packStatsEnvelope{}, fmt.Errorf("NFlow pack fold fileCount: %w", err)
	}
	funcCount, err := foldInteger(fields["funcCount"])
	if err != nil {
		return packStatsEnvelope{}, fmt.Errorf("NFlow pack fold funcCount: %w", err)
	}
	packedBytes, err := foldInteger(fields["packedBytes"])
	if err != nil {
		return packStatsEnvelope{}, fmt.Errorf("NFlow pack fold packedBytes: %w", err)
	}
	rawBytes, err := foldInteger(fields["rawBytes"])
	if err != nil {
		return packStatsEnvelope{}, fmt.Errorf("NFlow pack fold rawBytes: %w", err)
	}

	stats := PackStats{
		FileCount:   int(fileCount),
		FuncCount:   int(funcCount),
		PackedBytes: packedBytes,
		RawBytes:    rawBytes,
	}
	stats.EstimatedTokens = int64(float64(stats.PackedBytes) / 3.5)

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
		file, ok := raw.(ParsedFile)
		if !ok {
			return nil, fmt.Errorf("NFlow pack fold files[%d] has type %T, want operators.ParsedFile", i, raw)
		}
		files = append(files, file)
	}
	return files, nil
}

func parsedFilesFromRecords(records []any) ([]ParsedFile, error) {
	files := make([]ParsedFile, 0, len(records))
	for i, record := range records {
		file, ok := record.(ParsedFile)
		if !ok {
			return nil, fmt.Errorf("parsed-file stream item %d has type %T, want operators.ParsedFile", i, record)
		}
		files = append(files, file)
	}
	return files, nil
}
