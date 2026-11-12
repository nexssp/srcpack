package operators_test

import (
	"strings"
	"testing"

	"github.com/nexssp/srcpack/bundles/operators"
)

func TestMaskSecretsRecognizedCredentialShapes(t *testing.T) {
	repeat := func(char string, count int) string { return strings.Repeat(char, count) }
	tests := []struct {
		name        string
		input       string
		secret      string
		placeholder string
	}{
		{name: "anthropic api03", secret: "sk-ant-api03-" + repeat("F", 88), placeholder: "[REDACTED_SECRET]"},
		{name: "anthropic oat01", secret: "sk-ant-oat01-" + repeat("G", 88), placeholder: "[REDACTED_SECRET]"},
		{name: "openai legacy", secret: "sk-" + repeat("A", 36), placeholder: "[REDACTED_SECRET]"},
		{name: "openai project", secret: "sk-proj-" + repeat("B", 40), placeholder: "[REDACTED_SECRET]"},
		{name: "google api", secret: "AIzaSy" + repeat("C", 35), placeholder: "[REDACTED_SECRET]"},
		{name: "github ghu", secret: "ghu_" + repeat("D", 36), placeholder: "[REDACTED_SECRET]"},
		{name: "github ghs", secret: "ghs_" + repeat("E", 36), placeholder: "[REDACTED_SECRET]"},
		{name: "github ghr refresh", secret: "ghr_" + repeat("R", 36), placeholder: "[REDACTED_SECRET]"},
		{name: "github existing ghp", secret: "ghp_" + repeat("Q", 36), placeholder: "[REDACTED_SECRET]"},
		{name: "stripe live", secret: "sk_live_" + repeat("G", 24), placeholder: "[REDACTED_SECRET]"},
		{name: "stripe test", secret: "sk_test_" + repeat("T", 24), placeholder: "[REDACTED_SECRET]"},
		{name: "npm token", secret: "npm_" + repeat("H", 36), placeholder: "[REDACTED_SECRET]"},
		{name: "hugging face token", secret: "hf_" + repeat("J", 34), placeholder: "[REDACTED_SECRET]"},
		{name: "gitlab personal token", secret: "glpat-" + repeat("K", 20), placeholder: "[REDACTED_SECRET]"},
		{name: "jwt", secret: "eyJhbGciOiJmYWtlIn0.eyJzdWIiOiJmYWtlIn0." + repeat("L", 24), placeholder: "[REDACTED_SECRET]"},
		{name: "generic token assignment", input: `token = "` + repeat("m", 24) + `"`, secret: repeat("m", 24), placeholder: "[REDACTED_CREDENTIAL]"},
		{name: "generic api token assignment", input: `api_token: '` + repeat("n", 24) + `'`, secret: repeat("n", 24), placeholder: "[REDACTED_CREDENTIAL]"},
		{name: "generic access token assignment", input: `access_token = ` + repeat("p", 24), secret: repeat("p", 24), placeholder: "[REDACTED_CREDENTIAL]"},
		{name: "quoted json assignment", input: `{"access_token": "` + repeat("r", 24) + `"}`, secret: repeat("r", 24), placeholder: "[REDACTED_CREDENTIAL]"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.input
			if input == "" {
				input = tc.secret
			}
			got := operators.MaskSecrets(input)
			if strings.Contains(got, tc.secret) || !strings.Contains(got, tc.placeholder) {
				t.Fatalf("%s was not redacted as expected", tc.name)
			}
		})
	}
}

func TestMaskSecretsLeavesNearMissesAndImportPaths(t *testing.T) {
	repeat := func(char string, count int) string { return strings.Repeat(char, count) }
	tests := []struct {
		name  string
		input string
	}{
		{name: "anthropic-like import path", input: `import "github.com/acme/sk-ant-toolkit-very-long-package-name"`},
		{name: "short anthropic suffix", input: "sk-ant-api03-" + repeat("F", 79)},
		{name: "hyphenated project package path", input: `import "github.com/acme/sk-proj-this-is-a-very-long-package-name-for-a-library"`},
		{name: "token embedded in identifier", input: "prefix-sk-proj-" + repeat("B", 40) + "-suffix"},
		{name: "short openai-like value", input: "sk-" + repeat("A", 31)},
		{name: "short github value", input: "ghu_" + repeat("D", 35)},
		{name: "ordinary token assignment", input: `token: "short"`},
		{name: "key with token suffix", input: `my_token: "` + repeat("m", 24) + `"`},
		{name: "token assigned from function call", input: "token = getAuthTokenFromRequestContext(request)"},
		{name: "password assigned from validator call", input: "password = ValidatePasswordStrength(user)"},
		{name: "secret assigned from derivation call", input: "secret = deriveSecretFromConfig(cfg)"},
		{name: "token assigned from member reference", input: "token = userInfo.AccessToken"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := operators.MaskSecrets(tc.input); got != tc.input {
				t.Fatalf("%s was unexpectedly changed", tc.name)
			}
		})
	}
}
