package uploadsecurity

import (
	"bufio"
	"bytes"
	"image"
	"io"
	"unicode/utf8"

	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

const pdfTailScanBytes int64 = 4096

// ValidateContent performs the dedicated V1 content check for a canonical
// image, PDF or UTF-8 text type. The source is rewound before returning.
func ValidateContent(source io.ReadSeeker, canonicalType CanonicalType) (CanonicalType, error) {
	if source == nil {
		return "", NewError(CodeFileContentInvalid, nil)
	}

	size, err := source.Seek(0, io.SeekEnd)
	if err != nil {
		return "", NewError(CodeFileContentInvalid, err)
	}
	if size == 0 {
		_, _ = source.Seek(0, io.SeekStart)
		return "", NewError(CodeFileEmpty, nil)
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return "", NewError(CodeFileContentInvalid, err)
	}
	defer func() {
		_, _ = source.Seek(0, io.SeekStart)
	}()

	switch canonicalType {
	case TypeJPEG, TypePNG, TypeWebP:
		return validateImageContent(source, canonicalType)
	case TypePDF:
		return validatePDFContent(source, size)
	case TypeTXT, TypeCSV:
		return validateUTF8TextContent(source, canonicalType)
	default:
		return "", NewError(CodeFileTypeNotAllowed, nil)
	}
}

func validateImageContent(source io.Reader, canonicalType CanonicalType) (CanonicalType, error) {
	_, format, err := image.Decode(source)
	if err != nil {
		return "", NewError(CodeImageDecodeInvalid, err)
	}

	var decodedType CanonicalType
	switch format {
	case "jpeg":
		decodedType = TypeJPEG
	case "png":
		decodedType = TypePNG
	case "webp":
		decodedType = TypeWebP
	default:
		return "", NewError(CodeFileTypeNotAllowed, nil)
	}
	if decodedType != canonicalType {
		return "", NewError(CodeFileTypeMismatch, nil)
	}
	return canonicalType, nil
}

func validatePDFContent(source io.ReadSeeker, size int64) (CanonicalType, error) {
	header := make([]byte, len("%PDF-"))
	if _, err := io.ReadFull(source, header); err != nil || !bytes.Equal(header, []byte("%PDF-")) {
		return "", NewError(CodeFileContentInvalid, err)
	}

	tailSize := pdfTailScanBytes
	if size < tailSize {
		tailSize = size
	}
	if _, err := source.Seek(-tailSize, io.SeekEnd); err != nil {
		return "", NewError(CodeFileContentInvalid, err)
	}
	tail := make([]byte, tailSize)
	if _, err := io.ReadFull(source, tail); err != nil {
		return "", NewError(CodeFileContentInvalid, err)
	}
	if !bytes.HasSuffix(bytes.TrimSpace(tail), []byte("%%EOF")) {
		return "", NewError(CodeFileContentInvalid, nil)
	}
	return TypePDF, nil
}

func validateUTF8TextContent(source io.Reader, canonicalType CanonicalType) (CanonicalType, error) {
	reader := bufio.NewReader(source)
	prefix, _ := reader.Peek(3)
	if len(prefix) >= 2 &&
		((prefix[0] == 0xff && prefix[1] == 0xfe) ||
			(prefix[0] == 0xfe && prefix[1] == 0xff)) {
		return "", NewError(CodeFileEncodingInvalid, nil)
	}
	if len(prefix) == 3 && bytes.Equal(prefix, []byte{0xef, 0xbb, 0xbf}) {
		if _, err := reader.Discard(3); err != nil {
			return "", NewError(CodeFileEncodingInvalid, err)
		}
	}

	for {
		char, width, err := reader.ReadRune()
		if err == io.EOF {
			return canonicalType, nil
		}
		if err != nil {
			return "", NewError(CodeFileEncodingInvalid, err)
		}
		if char == 0 || (char == utf8.RuneError && width == 1) {
			return "", NewError(CodeFileEncodingInvalid, nil)
		}
	}
}
