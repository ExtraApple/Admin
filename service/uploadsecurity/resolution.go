package uploadsecurity

import (
	"path"
	"strings"
)

// TypeEvidence contains the independent type signals that must agree before
// an upload can be assigned a trusted canonical type.
type TypeEvidence struct {
	FileName      string
	DeclaredMIME  string
	DetectedMIME  string
	ValidatedType CanonicalType
}

// ResolveCanonicalType requires the filename extension, declared MIME,
// detected MIME and dedicated validator result to resolve to one V1 type.
func ResolveCanonicalType(evidence TypeEvidence) (TypeDefinition, error) {
	if err := ValidateNoDangerousDoubleExtension(evidence.FileName); err != nil {
		return TypeDefinition{}, err
	}

	fileName := path.Base(strings.ReplaceAll(evidence.FileName, `\`, "/"))
	extensionType, ok := LookupTypeByExtension(path.Ext(fileName))
	if !ok {
		return TypeDefinition{}, NewError(CodeFileTypeNotAllowed, nil)
	}

	declaredType, ok := LookupTypeByMIME(evidence.DeclaredMIME)
	if !ok {
		return TypeDefinition{}, NewError(CodeFileTypeNotAllowed, nil)
	}

	detectedType, ok := LookupTypeByMIME(evidence.DetectedMIME)
	if !ok {
		return TypeDefinition{}, NewError(CodeFileTypeNotAllowed, nil)
	}

	if evidence.ValidatedType == "" {
		return TypeDefinition{}, NewError(CodeFileContentInvalid, nil)
	}
	definition, ok := DefinitionForType(evidence.ValidatedType)
	if !ok {
		return TypeDefinition{}, NewError(CodeFileTypeNotAllowed, nil)
	}

	if extensionType != definition.Type ||
		declaredType != definition.Type ||
		detectedType != definition.Type {
		return TypeDefinition{}, NewError(CodeFileTypeMismatch, nil)
	}
	return definition, nil
}
