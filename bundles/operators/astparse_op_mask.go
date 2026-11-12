package operators

import (
	"bytes"
	"regexp"
	"strings"
)

var compositeSecretRegex = regexp.MustCompile(
	`(?s:(?P<privkey>-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----.*?-----END (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----))` +
		`|(?P<token>\b(?:AKIA[0-9A-Z]{16}|gh[posu]_[a-zA-Z0-9]{36}|github_pat_[a-zA-Z0-9_]{82}|xox[baprs]-[0-9a-zA-Z]{10,48}|sk-ant-(?:api0[0-9]|oat01)-[a-zA-Z0-9_-]{80,}|sk-proj-[a-zA-Z0-9_]{32,}|sk-[a-zA-Z0-9]{32,}|AIzaSy[a-zA-Z0-9_-]{30,}|sk_live_[a-zA-Z0-9]{20,}|npm_[a-zA-Z0-9]{20,}|hf_[a-zA-Z0-9]{20,}|glpat-[a-zA-Z0-9_-]{20,}|eyJ[a-zA-Z0-9_-]{5,}\.eyJ[a-zA-Z0-9_-]{5,}\.[a-zA-Z0-9_-]{5,})\b)` +
		`|(?i:(?P<bearer>bearer\s+)(?P<bearertok>[a-zA-Z0-9_\-\.]{25,}))` +
		`|(?P<assign_prefix>(?m:^|[^a-zA-Z0-9_-]))(?P<assign_key_open>["']?)(?i:(?P<assign_key>api[_-]?key|secret|password|passwd|auth[_-]?token|token|api[_-]?token|access[_-]?token))(?P<assign_key_close>["']?)(?:(?P<assign_sep>\s*[:=]\s*["'])(?P<assign_val>[^"'\s]{8,})(?P<assign_quote>["'])|(?P<assign_unquoted_sep>\s*[:=]\s*)(?P<assign_unquoted_val>[a-zA-Z0-9_./+=:-]{20,}))` +
		`|(?P<url_user>://[^:\s/@]+:)(?P<url_pass>[^@\s/]+)(?P<url_host>@[^/\s:]+)`,
)

var (
	privKeyIdx           = compositeSecretRegex.SubexpIndex("privkey")
	tokenIdx             = compositeSecretRegex.SubexpIndex("token")
	bearerIdx            = compositeSecretRegex.SubexpIndex("bearer")
	assignPrefixIdx      = compositeSecretRegex.SubexpIndex("assign_prefix")
	assignKeyOpenIdx     = compositeSecretRegex.SubexpIndex("assign_key_open")
	assignKeyIdx         = compositeSecretRegex.SubexpIndex("assign_key")
	assignKeyCloseIdx    = compositeSecretRegex.SubexpIndex("assign_key_close")
	assignSepIdx         = compositeSecretRegex.SubexpIndex("assign_sep")
	assignValIdx         = compositeSecretRegex.SubexpIndex("assign_val")
	assignQuoteIdx       = compositeSecretRegex.SubexpIndex("assign_quote")
	assignUnquotedSepIdx = compositeSecretRegex.SubexpIndex("assign_unquoted_sep")
	urlUserIdx           = compositeSecretRegex.SubexpIndex("url_user")
	urlPassIdx           = compositeSecretRegex.SubexpIndex("url_pass")
	urlHostIdx           = compositeSecretRegex.SubexpIndex("url_host")
)

func MaskSecretsFast(src []byte, dst *bytes.Buffer) {
	dst.Grow(len(src))
	lastIdx := 0

	matches := compositeSecretRegex.FindAllSubmatchIndex(src, -1)
	for _, m := range matches {
		if m[2*tokenIdx] >= 0 && !hasSecretTokenBoundaries(src, m[2*tokenIdx], m[2*tokenIdx+1]) {
			continue
		}
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
			dst.Write(src[m[2*assignPrefixIdx]:m[2*assignPrefixIdx+1]])
			dst.Write(src[m[2*assignKeyOpenIdx]:m[2*assignKeyOpenIdx+1]])
			dst.Write(src[m[2*assignKeyIdx]:m[2*assignKeyIdx+1]])
			dst.Write(src[m[2*assignKeyCloseIdx]:m[2*assignKeyCloseIdx+1]])
			if m[2*assignValIdx] >= 0 {
				dst.Write(src[m[2*assignSepIdx]:m[2*assignSepIdx+1]])
				dst.WriteString("[REDACTED_CREDENTIAL]")
				dst.Write(src[m[2*assignQuoteIdx]:m[2*assignQuoteIdx+1]])
			} else {
				dst.Write(src[m[2*assignUnquotedSepIdx]:m[2*assignUnquotedSepIdx+1]])
				dst.WriteString("[REDACTED_CREDENTIAL]")
			}
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

func hasSecretTokenBoundaries(src []byte, start, end int) bool {
	return (start == 0 || !isSecretTokenChar(src[start-1])) &&
		(end == len(src) || !isSecretTokenChar(src[end]))
}

func isSecretTokenChar(b byte) bool {
	return b == '_' || b == '-' ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9')
}

func MaskSecrets(input string) string {
	if input == "" {
		return ""
	}
	buf := bytes.NewBuffer(make([]byte, 0, len(input)))
	MaskSecretsFast([]byte(input), buf)
	return strings.Clone(buf.String())
}
