package operators

import "testing"

func TestIsConfigContextFile(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{path: ".golangci.yml", want: true},
		{path: "package.json", want: true},
		{path: "config/settings.toml", want: true},
		{path: "go.mod", want: true},
		{path: ".env", want: false},
		{path: ".env.example", want: false},
		{path: ".npmrc", want: false},
		{path: "credentials.json", want: false},
		{path: "service-account.json", want: false},
		{path: "private.pem", want: false},
		{path: "secrets/staging.yaml", want: false},
		{path: ".config/gcloud/application_default_credentials.json", want: false},
		{path: "secret.go", want: false},
		{path: "testdata/config.yaml", want: false},
		{path: "fixtures/config.json", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if got := isConfigContextFile(tc.path); got != tc.want {
				t.Errorf("isConfigContextFile(%q) = %t, want %t", tc.path, got, tc.want)
			}
		})
	}
}

func TestIsSensitiveSourceFileKeepsOrdinarySource(t *testing.T) {
	if isSensitiveSourceFile("secret.go") {
		t.Fatal("secret.go is source code, not a credential artifact")
	}
	if !isSensitiveSourceFile("credentials.json") {
		t.Fatal("credentials.json must be excluded")
	}
}
