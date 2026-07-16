package uploadsecurity

import (
	"bytes"
	"encoding/xml"
	"io"
	"net/url"
	"path"
	"strings"
)

const (
	contentTypesNamespace  = "http://schemas.openxmlformats.org/package/2006/content-types"
	relationshipsNamespace = "http://schemas.openxmlformats.org/package/2006/relationships"
	officeDocumentRelation = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	hyperlinkRelation      = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink"
	maxOOXMLXMLDepth       = 100
)

type ooxmlDefinition struct {
	mainPart          string
	mainRelationships string
	mainContentType   string
	rootName          string
	rootNamespace     string
}

var ooxmlDefinitions = map[CanonicalType]ooxmlDefinition{
	TypeDOCX: {
		mainPart:          "word/document.xml",
		mainRelationships: "word/_rels/document.xml.rels",
		mainContentType:   "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml",
		rootName:          "document",
		rootNamespace:     "http://schemas.openxmlformats.org/wordprocessingml/2006/main",
	},
	TypeXLSX: {
		mainPart:          "xl/workbook.xml",
		mainRelationships: "xl/_rels/workbook.xml.rels",
		mainContentType:   "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml",
		rootName:          "workbook",
		rootNamespace:     "http://schemas.openxmlformats.org/spreadsheetml/2006/main",
	},
	TypePPTX: {
		mainPart:          "ppt/presentation.xml",
		mainRelationships: "ppt/_rels/presentation.xml.rels",
		mainContentType:   "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml",
		rootName:          "presentation",
		rootNamespace:     "http://schemas.openxmlformats.org/presentationml/2006/main",
	},
}

// ValidateOOXML validates the required V1 package structure for DOCX, XLSX or
// PPTX and returns the confirmed canonical type.
func ValidateOOXML(
	source io.ReaderAt,
	size int64,
	expectedType CanonicalType,
) (CanonicalType, error) {
	definition, ok := ooxmlDefinitions[expectedType]
	if !ok {
		return "", NewError(CodeFileTypeNotAllowed, nil)
	}
	if isOLECompoundContainer(source, size) {
		return "", NewError(CodeOOXMLDangerousContent, nil)
	}

	archive, err := OpenRestrictedZIP(source, size, DefaultOOXMLZIPLimits())
	if err != nil {
		return "", err
	}

	if err := validateDangerousEntryNames(archive); err != nil {
		return "", err
	}
	if err := validateDangerousEntryContents(archive); err != nil {
		return "", err
	}
	if err := validateExclusiveBodyDirectory(archive, definition); err != nil {
		return "", err
	}
	if err := validateContentTypesEntry(archive, definition); err != nil {
		return "", err
	}
	if err := validatePackageRelationships(archive, definition); err != nil {
		return "", err
	}
	if err := validateMainPart(archive, definition); err != nil {
		return "", err
	}
	if err := validateMainRelationships(archive, definition); err != nil {
		return "", err
	}
	if err := validateAdditionalRelationships(archive, definition); err != nil {
		return "", err
	}
	return expectedType, nil
}

func isOLECompoundContainer(source io.ReaderAt, size int64) bool {
	if source == nil || size < 8 {
		return false
	}
	header := make([]byte, 8)
	if _, err := source.ReadAt(header, 0); err != nil {
		return false
	}
	return bytes.Equal(header, []byte{
		0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1,
	})
}

func validateDangerousEntryNames(archive *RestrictedZIP) error {
	for _, name := range archive.Names() {
		lowerName := strings.ToLower(name)
		baseName := path.Base(lowerName)
		switch baseName {
		case "vbaproject.bin", "encryptioninfo", "encryptedpackage":
			return NewError(CodeOOXMLDangerousContent, nil)
		}
		if strings.Contains(lowerName, "/activex/") ||
			strings.HasPrefix(lowerName, "activex/") ||
			strings.Contains(lowerName, "/embeddings/") ||
			strings.HasPrefix(lowerName, "embeddings/") {
			return NewError(CodeOOXMLDangerousContent, nil)
		}
		if _, dangerous := dangerousOOXMLEntryExtensions[path.Ext(baseName)]; dangerous {
			return NewError(CodeOOXMLDangerousContent, nil)
		}
	}
	return nil
}

var dangerousOOXMLEntryExtensions = map[string]struct{}{
	".app": {}, ".bat": {}, ".cmd": {}, ".com": {}, ".dll": {},
	".exe": {}, ".htm": {}, ".html": {}, ".jar": {}, ".js": {},
	".jse": {}, ".msi": {}, ".msp": {}, ".php": {}, ".ps1": {},
	".scr": {}, ".sh": {}, ".svg": {}, ".vbe": {}, ".vbs": {},
	".wsf": {},
}

func validateDangerousEntryContents(archive *RestrictedZIP) error {
	for _, name := range archive.Names() {
		reader, err := archive.Open(name)
		if err != nil {
			return err
		}

		prefix := make([]byte, 8)
		count, readErr := io.ReadFull(reader, prefix)
		_ = reader.Close()
		if readErr != nil &&
			readErr != io.EOF &&
			readErr != io.ErrUnexpectedEOF {
			if _, classified := CodeOf(readErr); classified {
				return readErr
			}
			return NewError(CodeOOXMLInvalid, readErr)
		}
		if hasDangerousEntrySignature(prefix[:count]) {
			return NewError(CodeOOXMLDangerousContent, nil)
		}
	}
	return nil
}

func hasDangerousEntrySignature(prefix []byte) bool {
	signatures := [][]byte{
		{'P', 'K', 3, 4},
		{'P', 'K', 5, 6},
		{'P', 'K', 7, 8},
		{'R', 'a', 'r', '!', 0x1a, 0x07},
		{'7', 'z', 0xbc, 0xaf, 0x27, 0x1c},
		{0x1f, 0x8b},
		{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1},
		{'M', 'Z'},
		{0x7f, 'E', 'L', 'F'},
		{0xfe, 0xed, 0xfa, 0xce},
		{0xfe, 0xed, 0xfa, 0xcf},
		{0xce, 0xfa, 0xed, 0xfe},
		{0xcf, 0xfa, 0xed, 0xfe},
		{'#', '!'},
	}
	for _, signature := range signatures {
		if bytes.HasPrefix(prefix, signature) {
			return true
		}
	}
	return false
}

func validateExclusiveBodyDirectory(
	archive *RestrictedZIP,
	expected ooxmlDefinition,
) error {
	expectedDirectory := strings.SplitN(expected.mainPart, "/", 2)[0] + "/"
	for _, name := range archive.Names() {
		for _, definition := range ooxmlDefinitions {
			directory := strings.SplitN(definition.mainPart, "/", 2)[0] + "/"
			if directory != expectedDirectory && strings.HasPrefix(name, directory) {
				return NewError(CodeOOXMLInvalid, nil)
			}
		}
	}
	return nil
}

func validateContentTypesEntry(archive *RestrictedZIP, definition ooxmlDefinition) error {
	return withRestrictedZIPEntry(archive, "[Content_Types].xml", func(source io.Reader) error {
		decoder := newRestrictedXMLDecoder(source)
		rootSeen := false
		mainTypeFound := false

		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return classifyXMLValidationError(err)
			}

			start, ok := token.(xml.StartElement)
			if !ok {
				continue
			}
			if !rootSeen {
				rootSeen = true
				if start.Name.Local != "Types" ||
					start.Name.Space != contentTypesNamespace {
					return NewError(CodeOOXMLInvalid, nil)
				}
				continue
			}
			partName := xmlAttribute(start.Attr, "PartName")
			contentType := xmlAttribute(start.Attr, "ContentType")
			if isDangerousOOXMLContentType(contentType) {
				return NewError(CodeOOXMLDangerousContent, nil)
			}
			if start.Name.Local != "Override" {
				continue
			}
			normalizedPartName := strings.TrimPrefix(partName, "/")
			for _, knownDefinition := range ooxmlDefinitions {
				if contentType != knownDefinition.mainContentType {
					continue
				}
				if contentType != definition.mainContentType ||
					normalizedPartName != definition.mainPart ||
					mainTypeFound {
					return NewError(CodeOOXMLInvalid, nil)
				}
				mainTypeFound = true
			}
		}

		if !rootSeen || !mainTypeFound {
			return NewError(CodeOOXMLInvalid, nil)
		}
		return nil
	})
}

func validatePackageRelationships(archive *RestrictedZIP, definition ooxmlDefinition) error {
	return withRestrictedZIPEntry(archive, "_rels/.rels", func(source io.Reader) error {
		relationships, err := parseRelationships(source)
		if err != nil {
			return err
		}
		if err := rejectDangerousRelationships(relationships); err != nil {
			return err
		}
		if err := rejectUnsafeExternalRelationships(relationships); err != nil {
			return err
		}

		found := 0
		for _, relationship := range relationships {
			if relationship.Type != officeDocumentRelation {
				continue
			}
			if relationship.TargetMode != "" ||
				strings.TrimPrefix(relationship.Target, "/") != definition.mainPart {
				return NewError(CodeOOXMLInvalid, nil)
			}
			found++
		}
		if found != 1 {
			return NewError(CodeOOXMLInvalid, nil)
		}
		return nil
	})
}

func validateMainPart(archive *RestrictedZIP, definition ooxmlDefinition) error {
	return withRestrictedZIPEntry(archive, definition.mainPart, func(source io.Reader) error {
		decoder := newRestrictedXMLDecoder(source)
		rootSeen := false
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return classifyXMLValidationError(err)
			}
			start, ok := token.(xml.StartElement)
			if !ok || rootSeen {
				continue
			}
			rootSeen = true
			if start.Name.Local != definition.rootName ||
				start.Name.Space != definition.rootNamespace {
				return NewError(CodeOOXMLInvalid, nil)
			}
		}
		if !rootSeen {
			return NewError(CodeOOXMLInvalid, nil)
		}
		return nil
	})
}

func validateMainRelationships(archive *RestrictedZIP, definition ooxmlDefinition) error {
	return withRestrictedZIPEntry(
		archive,
		definition.mainRelationships,
		func(source io.Reader) error {
			relationships, err := parseRelationships(source)
			if err != nil {
				return err
			}
			if err := rejectDangerousRelationships(relationships); err != nil {
				return err
			}
			return rejectUnsafeExternalRelationships(relationships)
		},
	)
}

func validateAdditionalRelationships(
	archive *RestrictedZIP,
	definition ooxmlDefinition,
) error {
	for _, name := range archive.Names() {
		if !strings.HasSuffix(strings.ToLower(name), ".rels") ||
			name == "_rels/.rels" ||
			name == definition.mainRelationships {
			continue
		}
		if err := withRestrictedZIPEntry(archive, name, func(source io.Reader) error {
			relationships, err := parseRelationships(source)
			if err != nil {
				return err
			}
			if err := rejectDangerousRelationships(relationships); err != nil {
				return err
			}
			return rejectUnsafeExternalRelationships(relationships)
		}); err != nil {
			return err
		}
	}
	return nil
}

type ooxmlRelationship struct {
	Type       string
	Target     string
	TargetMode string
}

func parseRelationships(source io.Reader) ([]ooxmlRelationship, error) {
	decoder := newRestrictedXMLDecoder(source)
	rootSeen := false
	var relationships []ooxmlRelationship

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, classifyXMLValidationError(err)
		}

		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if !rootSeen {
			rootSeen = true
			if start.Name.Local != "Relationships" ||
				start.Name.Space != relationshipsNamespace {
				return nil, NewError(CodeOOXMLInvalid, nil)
			}
			continue
		}
		if start.Name.Local == "Relationship" {
			relationships = append(relationships, ooxmlRelationship{
				Type:       xmlAttribute(start.Attr, "Type"),
				Target:     xmlAttribute(start.Attr, "Target"),
				TargetMode: xmlAttribute(start.Attr, "TargetMode"),
			})
		}
	}
	if !rootSeen {
		return nil, NewError(CodeOOXMLInvalid, nil)
	}
	return relationships, nil
}

type restrictedXMLDecoder struct {
	decoder *xml.Decoder
	depth   int
}

func newRestrictedXMLDecoder(source io.Reader) *restrictedXMLDecoder {
	return &restrictedXMLDecoder{decoder: xml.NewDecoder(source)}
}

func (decoder *restrictedXMLDecoder) Token() (xml.Token, error) {
	token, err := decoder.decoder.Token()
	if err != nil {
		return nil, err
	}

	switch token.(type) {
	case xml.Directive:
		return nil, NewError(CodeOOXMLInvalid, nil)
	case xml.StartElement:
		decoder.depth++
		if decoder.depth > maxOOXMLXMLDepth {
			return nil, NewError(CodeOOXMLResourceLimit, nil)
		}
	case xml.EndElement:
		decoder.depth--
	}
	return token, nil
}

func classifyXMLValidationError(err error) error {
	if _, classified := CodeOf(err); classified {
		return err
	}
	return NewError(CodeOOXMLInvalid, err)
}

func rejectDangerousRelationships(relationships []ooxmlRelationship) error {
	for _, relationship := range relationships {
		relationType := strings.ToLower(strings.TrimSpace(relationship.Type))
		for _, fragment := range []string{
			"/vbaproject",
			"/activex",
			"/oleobject",
			"/package",
			"/embeddedpackage",
			"/control",
		} {
			if strings.Contains(relationType, fragment) {
				return NewError(CodeOOXMLDangerousContent, nil)
			}
		}
	}
	return nil
}

func rejectUnsafeExternalRelationships(relationships []ooxmlRelationship) error {
	for _, relationship := range relationships {
		mode := strings.TrimSpace(relationship.TargetMode)
		target := strings.TrimSpace(relationship.Target)
		parsedTarget, parseErr := url.Parse(target)

		if mode == "" {
			if parseErr == nil && parsedTarget.Scheme != "" {
				return NewError(CodeOOXMLDangerousContent, nil)
			}
			continue
		}
		if !strings.EqualFold(mode, "External") ||
			relationship.Type != hyperlinkRelation ||
			parseErr != nil ||
			parsedTarget.Host == "" ||
			(!strings.EqualFold(parsedTarget.Scheme, "http") &&
				!strings.EqualFold(parsedTarget.Scheme, "https")) {
			return NewError(CodeOOXMLDangerousContent, nil)
		}
	}
	return nil
}

func isDangerousOOXMLContentType(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if contentType == "" {
		return false
	}
	for _, fragment := range []string{
		"macroenabled",
		"vbaproject",
		"activex",
		"oleobject",
		"encrypted",
	} {
		if strings.Contains(contentType, fragment) {
			return true
		}
	}
	switch contentType {
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return true
	default:
		return false
	}
}

func xmlAttribute(attributes []xml.Attr, name string) string {
	for _, attribute := range attributes {
		if attribute.Name.Local == name {
			return attribute.Value
		}
	}
	return ""
}

func withRestrictedZIPEntry(
	archive *RestrictedZIP,
	name string,
	validate func(io.Reader) error,
) error {
	reader, err := archive.Open(name)
	if err != nil {
		return err
	}
	defer reader.Close()
	return validate(reader)
}
