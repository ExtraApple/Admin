package uploadsecurity

import (
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxDisplayNameRunes = 255

// SanitizeAuditFileName returns a display-only filename suitable for audit
// metadata. Unlike upload validation, it does not require the extension to be
// on the V1 allowlist; it only removes paths and unsafe characters. An empty
// result means no safe filename could be derived.
func SanitizeAuditFileName(raw string, purpose Purpose) string {
	segment := path.Base(strings.ReplaceAll(raw, `\`, "/"))
	if segment == "" || segment == "." || segment == "/" {
		return ""
	}

	if extension, ok := normalizeCanonicalExtension(path.Ext(segment)); ok {
		name, err := SanitizeDisplayName(raw, purpose, extension)
		if err == nil {
			return name
		}
	}

	name := sanitizeDisplayNameBody(segment)
	runes := []rune(name)
	if len(runes) > maxDisplayNameRunes {
		name = strings.Trim(string(runes[:maxDisplayNameRunes]), " .")
	}
	return name
}

// SanitizeDisplayName returns a safe display-only filename while preserving
// the canonical extension selected by the validation policy.
func SanitizeDisplayName(raw string, purpose Purpose, canonicalExtension string) (string, error) {
	extension, ok := normalizeCanonicalExtension(canonicalExtension)
	if !ok {
		return "", NewError(CodeFileNameInvalid, nil)
	}

	segment := path.Base(strings.ReplaceAll(raw, `\`, "/"))
	if segment == "." || segment == "/" {
		segment = ""
	}
	if declaredExtension := path.Ext(segment); declaredExtension != "" {
		segment = strings.TrimSuffix(segment, declaredExtension)
	}

	body := sanitizeDisplayNameBody(segment)
	if body == "" {
		body = fallbackDisplayNameBody(purpose)
	}

	maxBodyRunes := maxDisplayNameRunes - utf8.RuneCountInString(extension)
	if maxBodyRunes < 1 {
		return "", NewError(CodeFileNameInvalid, nil)
	}
	bodyRunes := []rune(body)
	if len(bodyRunes) > maxBodyRunes {
		body = string(bodyRunes[:maxBodyRunes])
		body = strings.Trim(body, " .")
	}
	if body == "" {
		body = fallbackDisplayNameBody(purpose)
	}

	return body + extension, nil
}

// SanitizeManagedFileRename validates a client-supplied display-name body and
// appends the already trusted canonical extension. Paths, control/format
// characters, type-changing extensions, and dangerous double extensions are
// rejected instead of silently normalized.
func SanitizeManagedFileRename(rawBody, canonicalExtension string) (string, error) {
	extension, ok := normalizeCanonicalExtension(canonicalExtension)
	if !ok {
		return "", NewError(CodeFileNameInvalid, nil)
	}
	canonicalType, ok := LookupTypeByExtension(extension)
	if !ok {
		return "", NewError(CodeFileNameInvalid, nil)
	}
	if strings.ContainsAny(rawBody, `/\`) {
		return "", NewError(CodeFileNameInvalid, nil)
	}
	for _, char := range rawBody {
		if unicode.IsControl(char) || unicode.Is(unicode.Cf, char) || isBidiControl(char) {
			return "", NewError(CodeFileNameInvalid, nil)
		}
	}

	body := strings.TrimSpace(rawBody)
	if submittedExtension := path.Ext(body); submittedExtension != "" {
		if submittedType, known := LookupTypeByExtension(submittedExtension); known {
			if submittedType != canonicalType {
				return "", NewError(CodeFileNameInvalid, nil)
			}
			body = strings.TrimSuffix(body, submittedExtension)
		}
	}
	if sanitizeDisplayNameBody(body) == "" {
		return "", NewError(CodeFileNameInvalid, nil)
	}
	if err := ValidateNoDangerousDoubleExtension(body + extension); err != nil {
		return "", err
	}

	return SanitizeDisplayName(
		body+extension,
		PurposeManagedFile,
		extension,
	)
}

func normalizeCanonicalExtension(extension string) (string, bool) {
	extension = strings.ToLower(strings.TrimSpace(extension))
	if len(extension) < 2 || len(extension) > 16 || extension[0] != '.' {
		return "", false
	}
	for _, char := range extension[1:] {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') {
			return "", false
		}
	}
	return extension, true
}

func sanitizeDisplayNameBody(body string) string {
	var builder strings.Builder
	builder.Grow(len(body))
	previousWhitespace := false

	for _, char := range body {
		switch {
		case isBidiControl(char), char == '\r', char == '\n', unicode.IsControl(char):
			continue
		case unicode.IsSpace(char):
			if builder.Len() > 0 && !previousWhitespace {
				builder.WriteByte(' ')
				previousWhitespace = true
			}
		case isAllowedDisplayNameRune(char):
			builder.WriteRune(char)
			previousWhitespace = false
		}
	}

	return strings.Trim(builder.String(), " .")
}

func isAllowedDisplayNameRune(char rune) bool {
	if unicode.IsLetter(char) || unicode.IsNumber(char) || unicode.IsMark(char) {
		return true
	}
	switch char {
	case ' ', '-', '_', '.', '(', ')', '[', ']', '{', '}',
		'（', '）', '【', '】', '「', '」', '『', '』':
		return true
	default:
		return false
	}
}

func isBidiControl(char rune) bool {
	switch char {
	case '\u061c', '\u200e', '\u200f',
		'\u202a', '\u202b', '\u202c', '\u202d', '\u202e',
		'\u2066', '\u2067', '\u2068', '\u2069':
		return true
	default:
		return false
	}
}

func fallbackDisplayNameBody(purpose Purpose) string {
	if purpose == PurposeAvatar {
		return "avatar"
	}
	return "file"
}

var dangerousIntermediateExtensions = map[string]struct{}{
	"app": {}, "apk": {}, "bat": {}, "bash": {}, "cab": {}, "cgi": {},
	"cjs": {}, "class": {}, "cmd": {}, "com": {}, "dll": {}, "dmg": {},
	"doc": {}, "docm": {}, "dotm": {}, "dylib": {}, "exe": {}, "fish": {},
	"gz": {}, "htm": {}, "html": {}, "iso": {}, "jar": {}, "js": {},
	"jse": {}, "mjs": {}, "msi": {}, "msp": {}, "ole": {}, "php": {},
	"phar": {}, "phtml": {}, "pif": {}, "pl": {}, "potm": {}, "ppam": {},
	"ps1": {}, "psm1": {}, "ppt": {}, "pptm": {}, "py": {}, "pyw": {},
	"rar": {}, "rb": {}, "scr": {}, "sh": {}, "sldm": {}, "so": {},
	"svg": {}, "svgz": {}, "tar": {}, "tgz": {}, "vbe": {}, "vbs": {},
	"ws": {}, "wsf": {}, "wsh": {}, "xhtml": {}, "xlam": {}, "xls": {},
	"xlsm": {}, "xltm": {}, "xz": {}, "zip": {}, "7z": {}, "bz2": {},
}

// ValidateNoDangerousDoubleExtension rejects dangerous executable, script,
// active document and archive extensions that appear before the final
// extension.
func ValidateNoDangerousDoubleExtension(raw string) error {
	segment := path.Base(strings.ReplaceAll(raw, `\`, "/"))
	parts := strings.Split(segment, ".")
	if len(parts) < 3 {
		return nil
	}

	for _, part := range parts[1 : len(parts)-1] {
		extension := normalizeExtensionToken(part)
		if _, dangerous := dangerousIntermediateExtensions[extension]; dangerous {
			return NewError(CodeFileTypeNotAllowed, nil)
		}
	}
	return nil
}

func normalizeExtensionToken(value string) string {
	value = strings.TrimSpace(value)
	return strings.Map(func(char rune) rune {
		if isBidiControl(char) || unicode.IsControl(char) || unicode.Is(unicode.Cf, char) {
			return -1
		}
		return unicode.ToLower(char)
	}, value)
}
