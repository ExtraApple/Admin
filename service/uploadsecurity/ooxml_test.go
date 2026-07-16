package uploadsecurity_test

import (
	"bytes"
	"fmt"
	"testing"

	"admin/service/uploadsecurity"
)

func TestValidateOOXMLAcceptsRequiredDOCXXLSXAndPPTXStructures(t *testing.T) {
	for _, canonicalType := range []uploadsecurity.CanonicalType{
		uploadsecurity.TypeDOCX,
		uploadsecurity.TypeXLSX,
		uploadsecurity.TypePPTX,
	} {
		t.Run(string(canonicalType), func(t *testing.T) {
			content := buildZIPFixture(t, ooxmlFixtureEntries(t, canonicalType))
			got, err := uploadsecurity.ValidateOOXML(
				bytes.NewReader(content),
				int64(len(content)),
				canonicalType,
			)
			if err != nil {
				t.Fatalf("validate OOXML: %v", err)
			}
			if got != canonicalType {
				t.Fatalf("validated type: got %q, want %q", got, canonicalType)
			}
		})
	}
}

func TestValidateOOXMLRejectsMissingOrInvalidRequiredStructure(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string][]byte)
	}{
		{
			name: "missing content types",
			mutate: func(entries map[string][]byte) {
				delete(entries, "[Content_Types].xml")
			},
		},
		{
			name: "missing package relationships",
			mutate: func(entries map[string][]byte) {
				delete(entries, "_rels/.rels")
			},
		},
		{
			name: "missing main document",
			mutate: func(entries map[string][]byte) {
				delete(entries, "word/document.xml")
			},
		},
		{
			name: "missing main document relationships",
			mutate: func(entries map[string][]byte) {
				delete(entries, "word/_rels/document.xml.rels")
			},
		},
		{
			name: "content type does not declare main document",
			mutate: func(entries map[string][]byte) {
				entries["[Content_Types].xml"] = []byte(
					`<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"></Types>`,
				)
			},
		},
		{
			name: "package relationship targets another part",
			mutate: func(entries map[string][]byte) {
				entries["_rels/.rels"] = []byte(
					`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
						`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/other.xml"/>` +
						`</Relationships>`,
				)
			},
		},
		{
			name: "main document root is invalid",
			mutate: func(entries map[string][]byte) {
				entries["word/document.xml"] = []byte(
					`<?xml version="1.0"?><w:notDocument xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"/>`,
				)
			},
		},
		{
			name: "main relationships XML is malformed",
			mutate: func(entries map[string][]byte) {
				entries["word/_rels/document.xml.rels"] = []byte(`<Relationships>`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := ooxmlFixtureEntries(t, uploadsecurity.TypeDOCX)
			tt.mutate(entries)
			content := buildZIPFixture(t, entries)

			_, err := uploadsecurity.ValidateOOXML(
				bytes.NewReader(content),
				int64(len(content)),
				uploadsecurity.TypeDOCX,
			)
			code, ok := uploadsecurity.CodeOf(err)
			if !ok || code != uploadsecurity.CodeOOXMLInvalid {
				t.Fatalf("code: got %q, classified=%v", code, ok)
			}
		})
	}
}

func TestValidateOOXMLRejectsDisguisedOrMixedOfficePackages(t *testing.T) {
	docxMainType := "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	xlsxMainType := "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"

	tests := []struct {
		name         string
		entries      map[string][]byte
		expectedType uploadsecurity.CanonicalType
	}{
		{
			name: "ordinary ZIP disguised as DOCX",
			entries: map[string][]byte{
				"readme.txt": []byte("not an Office package"),
			},
			expectedType: uploadsecurity.TypeDOCX,
		},
		{
			name:         "DOCX disguised as XLSX",
			entries:      ooxmlFixtureEntries(t, uploadsecurity.TypeDOCX),
			expectedType: uploadsecurity.TypeXLSX,
		},
		{
			name: "DOCX carries another Office body directory",
			entries: func() map[string][]byte {
				entries := ooxmlFixtureEntries(t, uploadsecurity.TypeDOCX)
				entries["xl/workbook.xml"] = []byte(
					`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"/>`,
				)
				return entries
			}(),
			expectedType: uploadsecurity.TypeDOCX,
		},
		{
			name: "DOCX declares another Office main content type",
			entries: func() map[string][]byte {
				entries := ooxmlFixtureEntries(t, uploadsecurity.TypeDOCX)
				entries["[Content_Types].xml"] = []byte(fmt.Sprintf(
					`<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`+
						`<Override PartName="/word/document.xml" ContentType="%s"/>`+
						`<Override PartName="/xl/workbook.xml" ContentType="%s"/>`+
						`</Types>`,
					docxMainType,
					xlsxMainType,
				))
				return entries
			}(),
			expectedType: uploadsecurity.TypeDOCX,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := buildZIPFixture(t, tt.entries)
			_, err := uploadsecurity.ValidateOOXML(
				bytes.NewReader(content),
				int64(len(content)),
				tt.expectedType,
			)
			code, ok := uploadsecurity.CodeOf(err)
			if !ok || code != uploadsecurity.CodeOOXMLInvalid {
				t.Fatalf("code: got %q, classified=%v", code, ok)
			}
		})
	}
}

func TestOOXMLValidatorResultMustAgreeWithExtensionAndMIME(t *testing.T) {
	content := buildZIPFixture(t, ooxmlFixtureEntries(t, uploadsecurity.TypeDOCX))
	validatedType, err := uploadsecurity.ValidateOOXML(
		bytes.NewReader(content),
		int64(len(content)),
		uploadsecurity.TypeDOCX,
	)
	if err != nil {
		t.Fatalf("validate DOCX: %v", err)
	}

	tests := []uploadsecurity.TypeEvidence{
		{
			FileName:      "report.xlsx",
			DeclaredMIME:  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			DetectedMIME:  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			ValidatedType: validatedType,
		},
		{
			FileName:      "report.docx",
			DeclaredMIME:  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			DetectedMIME:  "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			ValidatedType: validatedType,
		},
	}

	for _, evidence := range tests {
		_, err := uploadsecurity.ResolveCanonicalType(evidence)
		code, ok := uploadsecurity.CodeOf(err)
		if !ok || code != uploadsecurity.CodeFileTypeMismatch {
			t.Fatalf("%+v code: got %q, classified=%v", evidence, code, ok)
		}
	}
}

func TestValidateOOXMLRejectsDangerousEntriesContentTypesAndRelationships(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string][]byte)
	}{
		{
			name: "VBA project entry",
			mutate: func(entries map[string][]byte) {
				entries["word/vbaProject.bin"] = []byte("macro")
			},
		},
		{
			name: "ActiveX entry",
			mutate: func(entries map[string][]byte) {
				entries["word/activeX/activeX1.xml"] = []byte("<activeX/>")
			},
		},
		{
			name: "OLE embedding entry",
			mutate: func(entries map[string][]byte) {
				entries["word/embeddings/oleObject1.bin"] = []byte("ole")
			},
		},
		{
			name: "script entry",
			mutate: func(entries map[string][]byte) {
				entries["word/media/payload.js"] = []byte("alert(1)")
			},
		},
		{
			name: "executable entry",
			mutate: func(entries map[string][]byte) {
				entries["word/media/payload.exe"] = []byte("MZ")
			},
		},
		{
			name: "encrypted package marker",
			mutate: func(entries map[string][]byte) {
				entries["EncryptedPackage"] = []byte("encrypted")
			},
		},
		{
			name: "macro content type",
			mutate: func(entries map[string][]byte) {
				entries["[Content_Types].xml"] = []byte(
					`<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
						`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
						`<Override PartName="/word/vbaProject.bin" ContentType="application/vnd.ms-office.vbaProject"/>` +
						`</Types>`,
				)
			},
		},
		{
			name: "macro relationship",
			mutate: func(entries map[string][]byte) {
				entries["word/_rels/document.xml.rels"] = []byte(
					`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
						`<Relationship Id="rId2" Type="http://schemas.microsoft.com/office/2006/relationships/vbaProject" Target="vbaProject.bin"/>` +
						`</Relationships>`,
				)
			},
		},
		{
			name: "dangerous relationship outside main relationships",
			mutate: func(entries map[string][]byte) {
				entries["word/_rels/header1.xml.rels"] = []byte(
					`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
						`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/oleObject" Target="../embeddings/oleObject1.bin"/>` +
						`</Relationships>`,
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := ooxmlFixtureEntries(t, uploadsecurity.TypeDOCX)
			tt.mutate(entries)
			content := buildZIPFixture(t, entries)

			_, err := uploadsecurity.ValidateOOXML(
				bytes.NewReader(content),
				int64(len(content)),
				uploadsecurity.TypeDOCX,
			)
			code, ok := uploadsecurity.CodeOf(err)
			if !ok || code != uploadsecurity.CodeOOXMLDangerousContent {
				t.Fatalf("code: got %q, classified=%v", code, ok)
			}
		})
	}
}

func TestValidateOOXMLRejectsEncryptedOLEContainer(t *testing.T) {
	content := []byte{
		0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1,
		0, 0, 0, 0,
	}

	_, err := uploadsecurity.ValidateOOXML(
		bytes.NewReader(content),
		int64(len(content)),
		uploadsecurity.TypeDOCX,
	)
	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != uploadsecurity.CodeOOXMLDangerousContent {
		t.Fatalf("code: got %q, classified=%v", code, ok)
	}
}

func TestValidateOOXMLRejectsDTDExternalEntitiesAndExcessiveDepth(t *testing.T) {
	deepMainDocument := bytes.NewBufferString(
		`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`,
	)
	for range 100 {
		deepMainDocument.WriteString("<w:node>")
	}
	for range 100 {
		deepMainDocument.WriteString("</w:node>")
	}
	deepMainDocument.WriteString("</w:document>")

	tests := []struct {
		name     string
		content  []byte
		wantCode uploadsecurity.Code
	}{
		{
			name: "DOCTYPE declaration",
			content: []byte(
				`<?xml version="1.0"?><!DOCTYPE document>` +
					`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"/>`,
			),
			wantCode: uploadsecurity.CodeOOXMLInvalid,
		},
		{
			name: "external entity declaration",
			content: []byte(
				`<?xml version="1.0"?><!DOCTYPE document [<!ENTITY xxe SYSTEM "file:///etc/passwd">]>` +
					`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"/>`,
			),
			wantCode: uploadsecurity.CodeOOXMLInvalid,
		},
		{
			name:     "XML nesting exceeds 100 levels",
			content:  deepMainDocument.Bytes(),
			wantCode: uploadsecurity.CodeOOXMLResourceLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := ooxmlFixtureEntries(t, uploadsecurity.TypeDOCX)
			entries["word/document.xml"] = tt.content
			content := buildZIPFixture(t, entries)

			_, err := uploadsecurity.ValidateOOXML(
				bytes.NewReader(content),
				int64(len(content)),
				uploadsecurity.TypeDOCX,
			)
			code, ok := uploadsecurity.CodeOf(err)
			if !ok || code != tt.wantCode {
				t.Fatalf("code: got %q, classified=%v, want %q", code, ok, tt.wantCode)
			}
		})
	}
}

func TestValidateOOXMLAllowsOnlyHTTPHyperlinkExternalRelationships(t *testing.T) {
	for _, target := range []string{
		"https://example.com/documentation",
		"http://example.com/reference",
	} {
		t.Run(target, func(t *testing.T) {
			entries := ooxmlFixtureEntries(t, uploadsecurity.TypeDOCX)
			entries["word/_rels/document.xml.rels"] = []byte(fmt.Sprintf(
				`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
					`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="%s" TargetMode="External"/>`+
					`</Relationships>`,
				target,
			))
			content := buildZIPFixture(t, entries)

			if _, err := uploadsecurity.ValidateOOXML(
				bytes.NewReader(content),
				int64(len(content)),
				uploadsecurity.TypeDOCX,
			); err != nil {
				t.Fatalf("validate OOXML: %v", err)
			}
		})
	}
}

func TestValidateOOXMLRejectsOtherExternalRelationships(t *testing.T) {
	tests := []struct {
		name         string
		relationType string
		target       string
		targetMode   string
	}{
		{
			name:         "external template",
			relationType: "http://schemas.openxmlformats.org/officeDocument/2006/relationships/attachedTemplate",
			target:       "https://example.com/template.dotx",
			targetMode:   "External",
		},
		{
			name:         "external image",
			relationType: "http://schemas.openxmlformats.org/officeDocument/2006/relationships/image",
			target:       "https://example.com/tracker.png",
			targetMode:   "External",
		},
		{
			name:         "file hyperlink",
			relationType: "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink",
			target:       "file:///etc/passwd",
			targetMode:   "External",
		},
		{
			name:         "FTP hyperlink",
			relationType: "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink",
			target:       "ftp://example.com/file",
			targetMode:   "External",
		},
		{
			name:         "JavaScript hyperlink",
			relationType: "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink",
			target:       "javascript:alert(1)",
			targetMode:   "External",
		},
		{
			name:         "external target omits TargetMode",
			relationType: "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink",
			target:       "https://example.com/hidden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := ooxmlFixtureEntries(t, uploadsecurity.TypeDOCX)
			entries["word/_rels/document.xml.rels"] = []byte(fmt.Sprintf(
				`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
					`<Relationship Id="rId2" Type="%s" Target="%s" TargetMode="%s"/>`+
					`</Relationships>`,
				tt.relationType,
				tt.target,
				tt.targetMode,
			))
			content := buildZIPFixture(t, entries)

			_, err := uploadsecurity.ValidateOOXML(
				bytes.NewReader(content),
				int64(len(content)),
				uploadsecurity.TypeDOCX,
			)
			code, ok := uploadsecurity.CodeOf(err)
			if !ok || code != uploadsecurity.CodeOOXMLDangerousContent {
				t.Fatalf("code: got %q, classified=%v", code, ok)
			}
		})
	}
}

func TestValidateOOXMLRejectsDangerousSignaturesHiddenByNeutralEntryNames(t *testing.T) {
	nestedZIP := buildZIPFixture(t, map[string][]byte{
		"payload.txt": []byte("nested"),
	})
	tests := []struct {
		name    string
		entry   string
		content []byte
	}{
		{
			name:    "nested ZIP in BIN entry",
			entry:   "word/media/blob.bin",
			content: nestedZIP,
		},
		{
			name:    "Windows executable in DAT entry",
			entry:   "word/media/blob.dat",
			content: []byte{'M', 'Z', 0, 0, 0, 0, 0, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := ooxmlFixtureEntries(t, uploadsecurity.TypeDOCX)
			entries[tt.entry] = tt.content
			content := buildZIPFixture(t, entries)

			_, err := uploadsecurity.ValidateOOXML(
				bytes.NewReader(content),
				int64(len(content)),
				uploadsecurity.TypeDOCX,
			)
			code, ok := uploadsecurity.CodeOf(err)
			if !ok || code != uploadsecurity.CodeOOXMLDangerousContent {
				t.Fatalf("code: got %q, classified=%v", code, ok)
			}
		})
	}
}

func ooxmlFixtureEntries(
	t *testing.T,
	canonicalType uploadsecurity.CanonicalType,
) map[string][]byte {
	t.Helper()

	var mainPart string
	var mainRelationships string
	var mainContentType string
	var rootName string
	var rootNamespace string

	switch canonicalType {
	case uploadsecurity.TypeDOCX:
		mainPart = "word/document.xml"
		mainRelationships = "word/_rels/document.xml.rels"
		mainContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
		rootName = "document"
		rootNamespace = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	case uploadsecurity.TypeXLSX:
		mainPart = "xl/workbook.xml"
		mainRelationships = "xl/_rels/workbook.xml.rels"
		mainContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"
		rootName = "workbook"
		rootNamespace = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	case uploadsecurity.TypePPTX:
		mainPart = "ppt/presentation.xml"
		mainRelationships = "ppt/_rels/presentation.xml.rels"
		mainContentType = "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"
		rootName = "presentation"
		rootNamespace = "http://schemas.openxmlformats.org/presentationml/2006/main"
	default:
		t.Fatalf("unsupported OOXML fixture type %q", canonicalType)
	}

	return map[string][]byte{
		"[Content_Types].xml": []byte(fmt.Sprintf(
			`<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`+
				`<Override PartName="/%s" ContentType="%s"/>`+
				`</Types>`,
			mainPart,
			mainContentType,
		)),
		"_rels/.rels": []byte(fmt.Sprintf(
			`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
				`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="%s"/>`+
				`</Relationships>`,
			mainPart,
		)),
		mainPart: []byte(fmt.Sprintf(
			`<?xml version="1.0"?><x:%s xmlns:x="%s"/>`,
			rootName,
			rootNamespace,
		)),
		mainRelationships: []byte(
			`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`,
		),
	}
}
