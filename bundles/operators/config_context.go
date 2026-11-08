package operators

import (
	"path/filepath"
	"strings"
)

var configContextExtensions = map[string]bool{
	".cfg": true, ".conf": true, ".ini": true,
	".json": true, ".json5": true, ".jsonc": true,
	".properties": true, ".toml": true, ".xml": true,
	".yaml": true, ".yml": true,
}

var configContextNames = map[string]bool{
	".dockerignore": true, ".editorconfig": true, ".gitattributes": true,
	".gitignore": true, ".nvmrc": true, ".prettierrc": true,
	".tool-versions": true, "dockerfile": true, "gnumakefile": true,
	"makefile": true, "go.mod": true, "go.work": true, "pipfile": true,
	"procfile": true, "requirements.txt": true, "taskfile": true,
}

var sensitiveConfigExtensions = map[string]bool{
	".cer": true, ".crt": true, ".jks": true, ".key": true,
	".keystore": true, ".p12": true, ".pfx": true, ".pem": true,
}

// isSensitiveSourceFile excludes common credential and private-key files even
// when they are explicitly selected. Secret masking remains a separate layer.
func isSensitiveSourceFile(path string) bool {
	clean := strings.ToLower(filepath.ToSlash(path))
	base := strings.ToLower(filepath.Base(path))
	if isSensitiveBaseName(base) {
		return true
	}
	ext := strings.ToLower(filepath.Ext(base))
	return sensitiveConfigExtensions[ext] || isSensitiveConfigName(base, ext) || isSensitiveConfigPath(clean, ext)
}

func isSensitiveBaseName(base string) bool {
	switch base {
	case ".npmrc", ".netrc", ".pypirc", "authorized_keys", "known_hosts",
		"credentials", "credential", "secrets", "secret", "token", "tokens":
		return true
	}
	return strings.HasPrefix(base, ".env") || strings.HasPrefix(base, "id_rsa") ||
		strings.HasPrefix(base, "id_ed25519") || strings.HasPrefix(base, "id_ecdsa") ||
		strings.HasPrefix(base, "id_dsa")
}

func isSensitiveConfigName(base, ext string) bool {
	if !configContextExtensions[ext] {
		return false
	}
	return strings.Contains(base, "credential") || strings.Contains(base, "secret") ||
		strings.Contains(base, "private-key") || strings.Contains(base, "private_key") ||
		strings.Contains(base, "privatekey") || strings.Contains(base, "service-account")
}

func isSensitiveConfigPath(path, ext string) bool {
	if hasPathComponent(path, ".aws") || hasPathComponent(path, ".azure") ||
		hasPathComponent(path, ".kube") || hasPathComponent(path, ".ssh") {
		return true
	}
	if configContextExtensions[ext] &&
		(hasPathComponent(path, "credentials") || hasPathComponent(path, "secrets")) {
		return true
	}
	return path == ".config/gcloud" || strings.HasPrefix(path, ".config/gcloud/") ||
		strings.HasSuffix(path, "/.config/gcloud") || strings.Contains(path, "/.config/gcloud/")
}

func isConfigContextFile(path string) bool {
	if strings.TrimSpace(path) == "" || isSensitiveSourceFile(path) {
		return false
	}
	clean := strings.ToLower(filepath.ToSlash(path))
	if hasPathComponent(clean, "fixtures") || hasPathComponent(clean, "testdata") {
		return false
	}
	base := strings.ToLower(filepath.Base(path))
	if configContextNames[base] {
		return true
	}
	return configContextExtensions[strings.ToLower(filepath.Ext(base))]
}

func sourceContextPath(file ParsedFile) string {
	if file.RelPath != "" {
		return file.RelPath
	}
	return file.Path
}

func hasPathComponent(path, component string) bool {
	for {
		part, rest, found := strings.Cut(path, "/")
		if part == component {
			return true
		}
		if !found {
			return false
		}
		path = rest
	}
}
