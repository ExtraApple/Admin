package uploadsecurity

import (
	"mime"
	"strings"
)

const (
	TypeJPEG CanonicalType = "jpeg"
	TypePNG  CanonicalType = "png"
	TypeWebP CanonicalType = "webp"
	TypePDF  CanonicalType = "pdf"
	TypeDOCX CanonicalType = "docx"
	TypeXLSX CanonicalType = "xlsx"
	TypePPTX CanonicalType = "pptx"
	TypeTXT  CanonicalType = "txt"
	TypeCSV  CanonicalType = "csv"
)

// TypeDefinition describes the trusted extension and MIME for a V1 type.
type TypeDefinition struct {
	Type               CanonicalType
	CanonicalExtension string
	Extensions         []string
	MIME               string
}

var typeDefinitions = map[CanonicalType]TypeDefinition{
	TypeJPEG: {TypeJPEG, ".jpg", []string{".jpg", ".jpeg"}, "image/jpeg"},
	TypePNG:  {TypePNG, ".png", []string{".png"}, "image/png"},
	TypeWebP: {TypeWebP, ".webp", []string{".webp"}, "image/webp"},
	TypePDF:  {TypePDF, ".pdf", []string{".pdf"}, "application/pdf"},
	TypeDOCX: {TypeDOCX, ".docx", []string{".docx"}, "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	TypeXLSX: {TypeXLSX, ".xlsx", []string{".xlsx"}, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	TypePPTX: {TypePPTX, ".pptx", []string{".pptx"}, "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
	TypeTXT:  {TypeTXT, ".txt", []string{".txt"}, "text/plain"},
	TypeCSV:  {TypeCSV, ".csv", []string{".csv"}, "text/csv"},
}

var typesByExtension = map[string]CanonicalType{
	".jpg":  TypeJPEG,
	".jpeg": TypeJPEG,
	".png":  TypePNG,
	".webp": TypeWebP,
	".pdf":  TypePDF,
	".docx": TypeDOCX,
	".xlsx": TypeXLSX,
	".pptx": TypePPTX,
	".txt":  TypeTXT,
	".csv":  TypeCSV,
}

var typesByMIME = map[string]CanonicalType{
	"image/jpeg":      TypeJPEG,
	"image/png":       TypePNG,
	"image/webp":      TypeWebP,
	"application/pdf": TypePDF,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   TypeDOCX,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         TypeXLSX,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": TypePPTX,
	"text/plain": TypeTXT,
	"text/csv":   TypeCSV,
}

// LookupTypeByExtension returns the V1 canonical type for an extension.
func LookupTypeByExtension(extension string) (CanonicalType, bool) {
	value, ok := typesByExtension[strings.ToLower(strings.TrimSpace(extension))]
	return value, ok
}

// LookupTypeByMIME returns the V1 canonical type for a declared or detected
// media type. MIME parameters do not change the base media type.
func LookupTypeByMIME(value string) (CanonicalType, bool) {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil {
		return "", false
	}
	canonicalType, ok := typesByMIME[strings.ToLower(mediaType)]
	return canonicalType, ok
}

// DefinitionForType returns a copy of the V1 canonical type definition.
func DefinitionForType(canonicalType CanonicalType) (TypeDefinition, bool) {
	definition, ok := typeDefinitions[canonicalType]
	if !ok {
		return TypeDefinition{}, false
	}
	definition.Extensions = append([]string(nil), definition.Extensions...)
	return definition, true
}
