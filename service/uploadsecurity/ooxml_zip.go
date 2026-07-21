package uploadsecurity

import (
	"archive/zip"
	"io"
	"path"
	"strings"
	"sync"
)

const (
	defaultOOXMLMaxEntries                = 2_000
	defaultOOXMLMaxEntryUncompressedBytes = 50 * 1024 * 1024
	defaultOOXMLMaxTotalUncompressedBytes = 200 * 1024 * 1024
	defaultOOXMLMaxEntryCompressionRatio  = 100
	defaultOOXMLMaxTotalCompressionRatio  = 100
)

// OOXMLZIPLimits bounds ZIP metadata and decompression work performed while
// validating an Office Open XML container.
type OOXMLZIPLimits struct {
	MaxEntries                int
	MaxEntryUncompressedBytes int64
	MaxTotalUncompressedBytes int64
	MaxEntryCompressionRatio  float64
	MaxTotalCompressionRatio  float64
}

// DefaultOOXMLZIPLimits returns the fixed V1 Office container limits.
func DefaultOOXMLZIPLimits() OOXMLZIPLimits {
	return OOXMLZIPLimits{
		MaxEntries:                defaultOOXMLMaxEntries,
		MaxEntryUncompressedBytes: defaultOOXMLMaxEntryUncompressedBytes,
		MaxTotalUncompressedBytes: defaultOOXMLMaxTotalUncompressedBytes,
		MaxEntryCompressionRatio:  defaultOOXMLMaxEntryCompressionRatio,
		MaxTotalCompressionRatio:  defaultOOXMLMaxTotalCompressionRatio,
	}
}

// RestrictedZIP exposes ZIP entries through streaming readers while enforcing
// both declared and observed decompression limits.
type RestrictedZIP struct {
	entries map[string]*zip.File
	names   []string
	limits  OOXMLZIPLimits

	mu             sync.Mutex
	totalBytesRead int64
}

// OpenRestrictedZIP opens a ZIP archive and rejects declared resource usage
// beyond the supplied limits before any entry is decompressed.
func OpenRestrictedZIP(
	source io.ReaderAt,
	size int64,
	limits OOXMLZIPLimits,
) (*RestrictedZIP, error) {
	limits = normalizeZIPLimits(limits)
	if source == nil || size < 1 ||
		limits.MaxEntries < 1 ||
		limits.MaxEntryUncompressedBytes < 1 ||
		limits.MaxTotalUncompressedBytes < 1 ||
		limits.MaxEntryCompressionRatio <= 0 ||
		limits.MaxTotalCompressionRatio <= 0 {
		return nil, NewError(CodeOOXMLInvalid, nil)
	}

	reader, err := zip.NewReader(source, size)
	if err != nil {
		return nil, NewError(CodeOOXMLInvalid, err)
	}
	if len(reader.File) > limits.MaxEntries {
		return nil, NewError(CodeOOXMLResourceLimit, nil)
	}

	archive := &RestrictedZIP{
		entries: make(map[string]*zip.File, len(reader.File)),
		names:   make([]string, 0, len(reader.File)),
		limits:  limits,
	}
	var declaredTotal uint64
	var declaredCompressedTotal uint64
	for _, entry := range reader.File {
		if err := validateZIPEntryName(entry.Name); err != nil {
			return nil, err
		}
		if _, exists := archive.entries[entry.Name]; exists {
			return nil, NewError(CodeOOXMLInvalid, nil)
		}
		if isNestedArchiveEntry(entry.Name) {
			return nil, NewError(CodeOOXMLDangerousContent, nil)
		}
		if entry.UncompressedSize64 > uint64(limits.MaxEntryUncompressedBytes) {
			return nil, NewError(CodeOOXMLResourceLimit, nil)
		}
		if entry.UncompressedSize64 > 0 {
			if entry.CompressedSize64 == 0 ||
				exceedsCompressionRatio(
					entry.UncompressedSize64,
					entry.CompressedSize64,
					limits.MaxEntryCompressionRatio,
				) {
				return nil, NewError(CodeOOXMLResourceLimit, nil)
			}
		}
		declaredTotal += entry.UncompressedSize64
		if declaredTotal > uint64(limits.MaxTotalUncompressedBytes) {
			return nil, NewError(CodeOOXMLResourceLimit, nil)
		}
		declaredCompressedTotal += entry.CompressedSize64
		archive.entries[entry.Name] = entry
		archive.names = append(archive.names, entry.Name)
	}
	if declaredTotal > 0 &&
		(declaredCompressedTotal == 0 ||
			exceedsCompressionRatio(
				declaredTotal,
				declaredCompressedTotal,
				limits.MaxTotalCompressionRatio,
			)) {
		return nil, NewError(CodeOOXMLResourceLimit, nil)
	}
	return archive, nil
}

func normalizeZIPLimits(limits OOXMLZIPLimits) OOXMLZIPLimits {
	if limits.MaxEntryCompressionRatio == 0 {
		limits.MaxEntryCompressionRatio = defaultOOXMLMaxEntryCompressionRatio
	}
	if limits.MaxTotalCompressionRatio == 0 {
		limits.MaxTotalCompressionRatio = defaultOOXMLMaxTotalCompressionRatio
	}
	return limits
}

func exceedsCompressionRatio(uncompressed, compressed uint64, maximum float64) bool {
	return float64(uncompressed) > float64(compressed)*maximum
}

func validateZIPEntryName(name string) error {
	if name == "" ||
		strings.ContainsRune(name, '\x00') ||
		strings.Contains(name, `\`) ||
		strings.HasPrefix(name, "/") ||
		(len(name) >= 2 && name[1] == ':') {
		return NewError(CodeOOXMLInvalid, nil)
	}

	segments := strings.Split(name, "/")
	for index, segment := range segments {
		if segment == "" {
			if index == len(segments)-1 {
				continue
			}
			return NewError(CodeOOXMLInvalid, nil)
		}
		if segment == "." || segment == ".." {
			return NewError(CodeOOXMLInvalid, nil)
		}
	}
	return nil
}

var nestedArchiveExtensions = map[string]struct{}{
	".7z": {}, ".apk": {}, ".bz2": {}, ".cab": {}, ".docx": {},
	".gz": {}, ".iso": {}, ".jar": {}, ".odp": {}, ".ods": {},
	".odt": {}, ".pptx": {}, ".rar": {}, ".tar": {}, ".tgz": {},
	".xlsx": {}, ".xz": {}, ".zip": {},
}

func isNestedArchiveEntry(name string) bool {
	extension := strings.ToLower(path.Ext(strings.TrimSuffix(name, "/")))
	_, nested := nestedArchiveExtensions[extension]
	return nested
}

// Names returns ZIP entry names in central-directory order.
func (archive *RestrictedZIP) Names() []string {
	if archive == nil {
		return nil
	}
	return append([]string(nil), archive.names...)
}

// Open returns a streaming reader whose observed decompression remains subject
// to the per-entry and archive-wide byte limits.
func (archive *RestrictedZIP) Open(name string) (io.ReadCloser, error) {
	if archive == nil {
		return nil, NewError(CodeOOXMLInvalid, nil)
	}
	entry, ok := archive.entries[name]
	if !ok {
		return nil, NewError(CodeOOXMLInvalid, nil)
	}
	reader, err := entry.Open()
	if err != nil {
		return nil, NewError(CodeOOXMLInvalid, err)
	}
	return &restrictedZIPEntryReader{
		archive: archive,
		reader:  reader,
	}, nil
}

type restrictedZIPEntryReader struct {
	archive   *RestrictedZIP
	reader    io.ReadCloser
	bytesRead int64
	failed    bool
}

func (reader *restrictedZIPEntryReader) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if reader.failed {
		return 0, NewError(CodeOOXMLResourceLimit, nil)
	}

	entryRemaining := reader.archive.limits.MaxEntryUncompressedBytes - reader.bytesRead
	reader.archive.mu.Lock()
	totalRemaining := reader.archive.limits.MaxTotalUncompressedBytes - reader.archive.totalBytesRead
	reader.archive.mu.Unlock()

	maxRead := int64(len(buffer))
	if entryRemaining < maxRead {
		maxRead = entryRemaining + 1
	}
	if totalRemaining < maxRead {
		maxRead = totalRemaining + 1
	}
	if maxRead < 1 {
		maxRead = 1
	}

	count, err := reader.reader.Read(buffer[:maxRead])
	if count == 0 {
		if err != nil && err != io.EOF {
			return 0, NewError(CodeOOXMLInvalid, err)
		}
		return 0, err
	}

	if reader.bytesRead+int64(count) > reader.archive.limits.MaxEntryUncompressedBytes {
		reader.failed = true
		return 0, NewError(CodeOOXMLResourceLimit, nil)
	}

	reader.archive.mu.Lock()
	if reader.archive.totalBytesRead+int64(count) >
		reader.archive.limits.MaxTotalUncompressedBytes {
		reader.archive.mu.Unlock()
		reader.failed = true
		return 0, NewError(CodeOOXMLResourceLimit, nil)
	}
	reader.archive.totalBytesRead += int64(count)
	reader.archive.mu.Unlock()

	reader.bytesRead += int64(count)
	if err != nil && err != io.EOF {
		return count, NewError(CodeOOXMLInvalid, err)
	}
	return count, err
}

func (reader *restrictedZIPEntryReader) Close() error {
	return reader.reader.Close()
}
