package uploadsecurity_test

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"testing"

	"admin/service/uploadsecurity"
)

func TestDefaultOOXMLZIPLimits(t *testing.T) {
	got := uploadsecurity.DefaultOOXMLZIPLimits()
	if got.MaxEntries != 2_000 {
		t.Fatalf("max entries: got %d", got.MaxEntries)
	}
	if got.MaxEntryUncompressedBytes != 50*1024*1024 {
		t.Fatalf("max entry bytes: got %d", got.MaxEntryUncompressedBytes)
	}
	if got.MaxTotalUncompressedBytes != 200*1024*1024 {
		t.Fatalf("max total bytes: got %d", got.MaxTotalUncompressedBytes)
	}
	if got.MaxEntryCompressionRatio != 100 {
		t.Fatalf("max entry compression ratio: got %v", got.MaxEntryCompressionRatio)
	}
	if got.MaxTotalCompressionRatio != 100 {
		t.Fatalf("max total compression ratio: got %v", got.MaxTotalCompressionRatio)
	}
}

func TestOpenRestrictedZIPEnforcesDeclaredResourceLimits(t *testing.T) {
	tests := []struct {
		name    string
		entries map[string][]byte
		limits  uploadsecurity.OOXMLZIPLimits
	}{
		{
			name: "entry count",
			entries: map[string][]byte{
				"1.xml": []byte("1"),
				"2.xml": []byte("2"),
				"3.xml": []byte("3"),
			},
			limits: uploadsecurity.OOXMLZIPLimits{
				MaxEntries:                2,
				MaxEntryUncompressedBytes: 16,
				MaxTotalUncompressedBytes: 16,
			},
		},
		{
			name: "single entry size",
			entries: map[string][]byte{
				"large.xml": []byte("12345"),
			},
			limits: uploadsecurity.OOXMLZIPLimits{
				MaxEntries:                2,
				MaxEntryUncompressedBytes: 4,
				MaxTotalUncompressedBytes: 16,
			},
		},
		{
			name: "total entry size",
			entries: map[string][]byte{
				"1.xml": []byte("123"),
				"2.xml": []byte("456"),
			},
			limits: uploadsecurity.OOXMLZIPLimits{
				MaxEntries:                2,
				MaxEntryUncompressedBytes: 4,
				MaxTotalUncompressedBytes: 5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := buildZIPFixture(t, tt.entries)
			_, err := uploadsecurity.OpenRestrictedZIP(
				bytes.NewReader(content),
				int64(len(content)),
				tt.limits,
			)
			code, ok := uploadsecurity.CodeOf(err)
			if !ok || code != uploadsecurity.CodeOOXMLResourceLimit {
				t.Fatalf("code: got %q, classified=%v", code, ok)
			}
		})
	}
}

func TestRestrictedZIPStreamsEntriesWithinLimits(t *testing.T) {
	content := buildZIPFixture(t, map[string][]byte{
		"[Content_Types].xml": []byte("<Types/>"),
		"word/document.xml":   []byte("<document/>"),
	})
	archive, err := uploadsecurity.OpenRestrictedZIP(
		bytes.NewReader(content),
		int64(len(content)),
		uploadsecurity.OOXMLZIPLimits{
			MaxEntries:                2,
			MaxEntryUncompressedBytes: 32,
			MaxTotalUncompressedBytes: 32,
		},
	)
	if err != nil {
		t.Fatalf("open restricted ZIP: %v", err)
	}

	names := archive.Names()
	if len(names) != 2 ||
		names[0] != "[Content_Types].xml" ||
		names[1] != "word/document.xml" {
		t.Fatalf("entry names: got %v", names)
	}

	reader, err := archive.Open("word/document.xml")
	if err != nil {
		t.Fatalf("open entry: %v", err)
	}
	defer reader.Close()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read entry: %v", err)
	}
	if string(got) != "<document/>" {
		t.Fatalf("entry content: got %q", got)
	}
}

func TestOpenRestrictedZIPEnforcesCompressionRatioLimits(t *testing.T) {
	compressible := bytes.Repeat([]byte("A"), 4*1024)
	normalLimits := uploadsecurity.OOXMLZIPLimits{
		MaxEntries:                4,
		MaxEntryUncompressedBytes: 16 * 1024,
		MaxTotalUncompressedBytes: 32 * 1024,
		MaxEntryCompressionRatio:  100,
		MaxTotalCompressionRatio:  100,
	}

	tests := []struct {
		name    string
		content []byte
		limits  uploadsecurity.OOXMLZIPLimits
	}{
		{
			name:    "single entry compression ratio",
			content: buildZIPFixture(t, map[string][]byte{"large.xml": compressible}),
			limits: uploadsecurity.OOXMLZIPLimits{
				MaxEntries:                4,
				MaxEntryUncompressedBytes: 16 * 1024,
				MaxTotalUncompressedBytes: 32 * 1024,
				MaxEntryCompressionRatio:  2,
				MaxTotalCompressionRatio:  1_000,
			},
		},
		{
			name: "total compression ratio",
			content: buildZIPFixture(t, map[string][]byte{
				"1.xml": compressible,
				"2.xml": compressible,
			}),
			limits: uploadsecurity.OOXMLZIPLimits{
				MaxEntries:                4,
				MaxEntryUncompressedBytes: 16 * 1024,
				MaxTotalUncompressedBytes: 32 * 1024,
				MaxEntryCompressionRatio:  1_000,
				MaxTotalCompressionRatio:  2,
			},
		},
		{
			name: "zero compressed size with non-empty content",
			content: setFirstCentralDirectoryCompressedSize(
				t,
				buildZIPFixture(t, map[string][]byte{"large.xml": []byte("not empty")}),
				0,
			),
			limits: normalLimits,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := uploadsecurity.OpenRestrictedZIP(
				bytes.NewReader(tt.content),
				int64(len(tt.content)),
				tt.limits,
			)
			code, ok := uploadsecurity.CodeOf(err)
			if !ok || code != uploadsecurity.CodeOOXMLResourceLimit {
				t.Fatalf("code: got %q, classified=%v", code, ok)
			}
		})
	}
}

func TestOpenRestrictedZIPRejectsUnsafeEntryNamesAndNestedArchives(t *testing.T) {
	limits := uploadsecurity.OOXMLZIPLimits{
		MaxEntries:                8,
		MaxEntryUncompressedBytes: 1024,
		MaxTotalUncompressedBytes: 4096,
	}
	tests := []struct {
		name     string
		entries  []zipFixtureEntry
		wantCode uploadsecurity.Code
	}{
		{
			name: "duplicate entry",
			entries: []zipFixtureEntry{
				{name: "word/document.xml", content: []byte("<one/>")},
				{name: "word/document.xml", content: []byte("<two/>")},
			},
			wantCode: uploadsecurity.CodeOOXMLInvalid,
		},
		{
			name: "absolute path",
			entries: []zipFixtureEntry{
				{name: "/word/document.xml", content: []byte("<document/>")},
			},
			wantCode: uploadsecurity.CodeOOXMLInvalid,
		},
		{
			name: "drive path",
			entries: []zipFixtureEntry{
				{name: "C:/word/document.xml", content: []byte("<document/>")},
			},
			wantCode: uploadsecurity.CodeOOXMLInvalid,
		},
		{
			name: "backslash path",
			entries: []zipFixtureEntry{
				{name: `word\document.xml`, content: []byte("<document/>")},
			},
			wantCode: uploadsecurity.CodeOOXMLInvalid,
		},
		{
			name: "parent traversal",
			entries: []zipFixtureEntry{
				{name: "word/../evil.xml", content: []byte("<evil/>")},
			},
			wantCode: uploadsecurity.CodeOOXMLInvalid,
		},
		{
			name: "nested archive",
			entries: []zipFixtureEntry{
				{name: "word/media/archive.zip", content: []byte("PK\x03\x04")},
			},
			wantCode: uploadsecurity.CodeOOXMLDangerousContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := buildZIPEntriesFixture(t, tt.entries)
			_, err := uploadsecurity.OpenRestrictedZIP(
				bytes.NewReader(content),
				int64(len(content)),
				limits,
			)
			code, ok := uploadsecurity.CodeOf(err)
			if !ok || code != tt.wantCode {
				t.Fatalf("code: got %q, classified=%v, want %q", code, ok, tt.wantCode)
			}
		})
	}
}

func TestOpenRestrictedZIPRejectsCorruptCentralDirectory(t *testing.T) {
	content := buildZIPFixture(t, map[string][]byte{
		"word/document.xml": []byte("<document/>"),
	})
	content = content[:len(content)-10]

	_, err := uploadsecurity.OpenRestrictedZIP(
		bytes.NewReader(content),
		int64(len(content)),
		uploadsecurity.OOXMLZIPLimits{
			MaxEntries:                2,
			MaxEntryUncompressedBytes: 1024,
			MaxTotalUncompressedBytes: 1024,
		},
	)
	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != uploadsecurity.CodeOOXMLInvalid {
		t.Fatalf("code: got %q, classified=%v", code, ok)
	}
}

type zipFixtureEntry struct {
	name    string
	content []byte
}

func buildZIPFixture(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()

	var orderedEntries []zipFixtureEntry
	for index := 0; ; index++ {
		name := orderedFixtureName(entries, index)
		if name == "" {
			break
		}
		orderedEntries = append(orderedEntries, zipFixtureEntry{
			name:    name,
			content: entries[name],
		})
	}
	return buildZIPEntriesFixture(t, orderedEntries)
}

func buildZIPEntriesFixture(t *testing.T, entries []zipFixtureEntry) []byte {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, fixture := range entries {
		entry, err := writer.Create(fixture.name)
		if err != nil {
			t.Fatalf("create ZIP entry %q: %v", fixture.name, err)
		}
		if _, err := entry.Write(fixture.content); err != nil {
			t.Fatalf("write ZIP entry %q: %v", fixture.name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close ZIP fixture: %v", err)
	}
	return buffer.Bytes()
}

func setFirstCentralDirectoryCompressedSize(t *testing.T, content []byte, size uint32) []byte {
	t.Helper()

	content = append([]byte(nil), content...)
	offset := bytes.Index(content, []byte{'P', 'K', 1, 2})
	if offset < 0 {
		t.Fatal("ZIP fixture has no central directory entry")
	}
	binary.LittleEndian.PutUint32(content[offset+20:offset+24], size)
	return content
}

func orderedFixtureName(entries map[string][]byte, index int) string {
	preferred := []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"word/document.xml",
		"word/_rels/document.xml.rels",
		"xl/workbook.xml",
		"xl/_rels/workbook.xml.rels",
		"ppt/presentation.xml",
		"ppt/_rels/presentation.xml.rels",
		"1.xml",
		"2.xml",
		"3.xml",
		"large.xml",
	}
	var names []string
	for _, name := range preferred {
		if _, ok := entries[name]; ok {
			names = append(names, name)
		}
	}
	for name := range entries {
		found := false
		for _, existing := range names {
			if existing == name {
				found = true
				break
			}
		}
		if !found {
			names = append(names, name)
		}
	}
	if index >= len(names) {
		return ""
	}
	if names[index] == "" {
		panic(fmt.Sprintf("empty ZIP fixture entry at index %d", index))
	}
	return names[index]
}
