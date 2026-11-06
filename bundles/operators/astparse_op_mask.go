package operators

import (
	"bytes"
	"regexp"
	"strings"
)

var compositeSecretRegex = regexp.MustCompile(
	`(?s:(?P<privkey>-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----.*?-----END (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----))` +
		`|(?P<token>\b(?:AKIA[0-9A-Z]{16}|ghp_[a-zA-Z0-9]{36}|gho_[a-zA-Z0-9]{36}|github_pat_[a-zA-Z0-9_]{82}|xox[baprs]-[0-9a-zA-Z]{10,48})\b)` +
		`|(?i:(?P<bearer>bearer\s+)(?P<bearertok>[a-zA-Z0-9_\-\.]{25,}))` +
		`|(?i:(?P<assign_key>api[_-]?key|secret|password|passwd|auth[_-]?token)(?P<assign_sep>\s*[:=]\s*["'])(?P<assign_val>[^"'\s]{8,})(?P<assign_quote>["']))` +
		`|(?P<url_user>://[^:\s/@]+:)(?P<url_pass>[^@\s/]+)(?P<url_host>@[^/\s:]+)`,
)

var (
	privKeyIdx   = compositeSecretRegex.SubexpIndex("privkey")
	tokenIdx     = compositeSecretRegex.SubexpIndex("token")
	bearerIdx    = compositeSecretRegex.SubexpIndex("bearer")
	assignKeyIdx = compositeSecretRegex.SubexpIndex("assign_key")
	assignSepIdx = compositeSecretRegex.SubexpIndex("assign_sep")
	assignQuoIdx = compositeSecretRegex.SubexpIndex("assign_quote")
	urlUserIdx   = compositeSecretRegex.SubexpIndex("url_user")
	urlPassIdx   = compositeSecretRegex.SubexpIndex("url_pass")
	urlHostIdx   = compositeSecretRegex.SubexpIndex("url_host")
)

func MaskSecretsFast(src []byte, dst *bytes.Buffer) {
	dst.Grow(len(src))
	lastIdx := 0

	matches := compositeSecretRegex.FindAllSubmatchIndex(src, -1)
	for _, m := range matches {
		dst.Write(src[lastIdx:m[0]])

		switch {
		case m[2*privKeyIdx] >= 0:
			dst.WriteString("[REDACTED_KEY]")
		case m[2*tokenIdx] >= 0:
			dst.WriteString("[REDACTED_SECRET]")
		case m[2*bearerIdx] >= 0:
			dst.Write(src[m[2*bearerIdx]:m[2*bearerIdx+1]])
			dst.WriteString("[REDACTED_TOKEN]")
		case m[2*assignKeyIdx] >= 0:
			dst.Write(src[m[2*assignKeyIdx]:m[2*assignKeyIdx+1]])
			dst.Write(src[m[2*assignSepIdx]:m[2*assignSepIdx+1]])
			dst.WriteString("[REDACTED_CREDENTIAL]")
			dst.Write(src[m[2*assignQuoIdx]:m[2*assignQuoIdx+1]])
		case m[2*urlPassIdx] >= 0:
			dst.Write(src[m[2*urlUserIdx]:m[2*urlUserIdx+1]])
			dst.WriteString("[REDACTED_CREDENTIAL]")
			dst.Write(src[m[2*urlHostIdx]:m[2*urlHostIdx+1]])
		default:
			dst.WriteString("[REDACTED_SECRET]")
		}
		lastIdx = m[1]
	}
	dst.Write(src[lastIdx:])
}

func MaskSecrets(input string) string {
	if input == "" {
		return ""
	}
	buf := bytes.NewBuffer(make([]byte, 0, len(input)))
	MaskSecretsFast([]byte(input), buf)
	return strings.Clone(buf.String())
}
