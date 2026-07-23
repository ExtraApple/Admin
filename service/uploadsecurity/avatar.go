package uploadsecurity

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"path"
	"strings"

	xdraw "golang.org/x/image/draw"
)

const (
	maxAvatarSidePixels  = 8192
	maxAvatarTotalPixels = 40_000_000
	maxAvatarOutputSide  = 1024
)

// AvatarProbe contains trusted image type and dimensions read from an avatar
// header without allocating the complete pixel buffer.
type AvatarProbe struct {
	CanonicalType CanonicalType
	Width         int
	Height        int
}

// AvatarOutput is the trusted result produced by avatar normalization.
type AvatarOutput struct {
	InputType     CanonicalType
	CanonicalType CanonicalType
	Extension     string
	MIME          string
	Width         int
	Height        int
	Data          []byte
}

type avatarValidator struct{}

// NewAvatarValidator returns the V1 validator that converts supported static
// avatar inputs into trusted JPEG or PNG output.
func NewAvatarValidator() Validator {
	return avatarValidator{}
}

func (avatarValidator) Validate(
	_ context.Context,
	input Input,
) (Result, error) {
	if input.Purpose != PurposeAvatar {
		return Result{}, NewError(CodeUploadBodyInvalid, nil)
	}
	if input.MaxBytes <= 0 || input.Size > input.MaxBytes {
		return Result{}, NewError(CodeFileTooLarge, nil)
	}
	if err := ValidateExtensionChain(input.FileName); err != nil {
		return Result{}, err
	}

	fileName := path.Base(strings.ReplaceAll(input.FileName, `\`, "/"))
	expectedType, ok := LookupTypeByExtension(path.Ext(fileName))
	if !ok || !isAvatarInputType(expectedType) {
		return Result{}, NewError(CodeFileTypeNotAllowed, nil)
	}
	declaredType, ok := LookupTypeByMIME(input.DeclaredMIME)
	if !ok || !isAvatarInputType(declaredType) {
		return Result{}, NewError(CodeFileTypeNotAllowed, nil)
	}
	if declaredType != expectedType {
		return Result{}, NewError(CodeFileTypeMismatch, nil)
	}
	if input.Reader == nil {
		return Result{}, NewError(CodeUploadBodyInvalid, nil)
	}

	content, err := io.ReadAll(io.LimitReader(input.Reader, input.MaxBytes+1))
	if err != nil {
		return Result{}, NewError(CodeUploadBodyInvalid, err)
	}
	if int64(len(content)) > input.MaxBytes {
		return Result{}, NewError(CodeFileTooLarge, nil)
	}
	if len(content) == 0 {
		return Result{}, NewError(CodeFileEmpty, nil)
	}
	if input.Size >= 0 && input.Size != int64(len(content)) {
		return Result{}, NewError(CodeUploadBodyInvalid, nil)
	}

	probe, err := ProbeAvatarHeader(content)
	if err != nil {
		return Result{}, err
	}
	if probe.CanonicalType != expectedType {
		return Result{}, NewError(CodeFileTypeMismatch, nil)
	}
	inputDefinition, ok := DefinitionForType(probe.CanonicalType)
	if !ok {
		return Result{}, NewError(CodeFileTypeNotAllowed, nil)
	}

	normalized, err := NormalizeAvatar(content, expectedType, input.MaxBytes)
	if err != nil {
		return Result{}, err
	}
	displayName, err := SanitizeDisplayName(
		input.FileName,
		PurposeAvatar,
		normalized.Extension,
	)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Purpose:            PurposeAvatar,
		FileName:           displayName,
		CanonicalType:      normalized.CanonicalType,
		CanonicalExtension: normalized.Extension,
		CanonicalMIME:      normalized.MIME,
		DetectedMIME:       inputDefinition.MIME,
		Size:               int64(len(normalized.Data)),
		ContentSHA256:      SHA256Hex(normalized.Data),
		PolicyVersion:      PolicyVersionV1,
		Reader:             bytes.NewReader(normalized.Data),
	}, nil
}

func isAvatarInputType(canonicalType CanonicalType) bool {
	switch canonicalType {
	case TypeJPEG, TypePNG, TypeWebP:
		return true
	default:
		return false
	}
}

// ProbeAvatarHeader reads only enough image data to identify a supported
// avatar and reject dimensions that would make a full decode unsafe.
func ProbeAvatarHeader(content []byte) (AvatarProbe, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return AvatarProbe{}, NewError(CodeImageDecodeInvalid, err)
	}

	canonicalType, ok := canonicalImageType(format)
	if !ok {
		return AvatarProbe{}, NewError(CodeFileTypeNotAllowed, nil)
	}
	if config.Width <= 0 || config.Height <= 0 {
		return AvatarProbe{}, NewError(CodeImageDecodeInvalid, nil)
	}
	if config.Width > maxAvatarSidePixels ||
		config.Height > maxAvatarSidePixels ||
		uint64(config.Width)*uint64(config.Height) > maxAvatarTotalPixels {
		return AvatarProbe{}, NewError(CodeImageDimensionLimit, nil)
	}

	return AvatarProbe{
		CanonicalType: canonicalType,
		Width:         config.Width,
		Height:        config.Height,
	}, nil
}

// NormalizeAvatar validates and standardizes a supported static avatar.
func NormalizeAvatar(content []byte, expectedType CanonicalType, maxOutputBytes int64) (AvatarOutput, error) {
	probe, err := ProbeAvatarHeader(content)
	if err != nil {
		return AvatarOutput{}, err
	}
	if _, ok := canonicalImageType(string(expectedType)); !ok {
		return AvatarOutput{}, NewError(CodeFileTypeNotAllowed, nil)
	}
	if probe.CanonicalType != expectedType {
		return AvatarOutput{}, NewError(CodeFileTypeMismatch, nil)
	}

	animated, err := hasAvatarAnimation(content, probe.CanonicalType)
	if err != nil {
		return AvatarOutput{}, NewError(CodeImageDecodeInvalid, err)
	}
	if animated {
		return AvatarOutput{}, NewError(CodeImageDecodeInvalid, nil)
	}

	decoded, format, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		return AvatarOutput{}, NewError(CodeImageDecodeInvalid, err)
	}
	decodedType, ok := canonicalImageType(format)
	if !ok {
		return AvatarOutput{}, NewError(CodeFileTypeNotAllowed, nil)
	}
	if decodedType != expectedType {
		return AvatarOutput{}, NewError(CodeFileTypeMismatch, nil)
	}

	bounds := decoded.Bounds()
	if bounds.Dx() != probe.Width || bounds.Dy() != probe.Height {
		return AvatarOutput{}, NewError(CodeImageDecodeInvalid, nil)
	}
	normalized := normalizeAvatarPixels(decoded, avatarOrientation(content, decodedType))
	bounds = normalized.Bounds()

	outputType := TypeJPEG
	var buffer bytes.Buffer
	if hasAvatarTransparency(normalized) {
		outputType = TypePNG
		err = png.Encode(&buffer, normalized)
	} else {
		err = jpeg.Encode(&buffer, normalized, &jpeg.Options{Quality: 85})
	}
	if err != nil {
		return AvatarOutput{}, NewError(CodeImageDecodeInvalid, err)
	}
	if maxOutputBytes <= 0 || int64(buffer.Len()) > maxOutputBytes {
		return AvatarOutput{}, NewError(CodeFileTooLarge, nil)
	}
	definition, ok := DefinitionForType(outputType)
	if !ok {
		return AvatarOutput{}, NewError(CodeInternalError, nil)
	}

	return AvatarOutput{
		InputType:     decodedType,
		CanonicalType: outputType,
		Extension:     definition.CanonicalExtension,
		MIME:          definition.MIME,
		Width:         bounds.Dx(),
		Height:        bounds.Dy(),
		Data:          buffer.Bytes(),
	}, nil
}

func canonicalImageType(format string) (CanonicalType, bool) {
	switch format {
	case "jpeg":
		return TypeJPEG, true
	case "png":
		return TypePNG, true
	case "webp":
		return TypeWebP, true
	default:
		return "", false
	}
}

func hasAvatarAnimation(content []byte, canonicalType CanonicalType) (bool, error) {
	switch canonicalType {
	case TypePNG:
		return hasPNGAnimation(content)
	case TypeWebP:
		return hasWebPAnimation(content)
	default:
		return false, nil
	}
}

func hasPNGAnimation(content []byte) (bool, error) {
	const pngSignatureSize = 8
	if len(content) < pngSignatureSize {
		return false, errors.New("truncated PNG")
	}

	for offset := pngSignatureSize; offset < len(content); {
		if len(content)-offset < 12 {
			return false, errors.New("truncated PNG chunk")
		}
		length := uint64(binary.BigEndian.Uint32(content[offset : offset+4]))
		chunkSize := uint64(12) + length
		if chunkSize > uint64(len(content)-offset) {
			return false, errors.New("invalid PNG chunk size")
		}
		chunkType := string(content[offset+4 : offset+8])
		if chunkType == "acTL" {
			return true, nil
		}
		offset += int(chunkSize)
		if chunkType == "IEND" {
			return false, nil
		}
	}
	return false, errors.New("missing PNG end chunk")
}

func hasWebPAnimation(content []byte) (bool, error) {
	if len(content) < 12 ||
		string(content[0:4]) != "RIFF" ||
		string(content[8:12]) != "WEBP" {
		return false, errors.New("invalid WebP container")
	}

	for offset := 12; offset < len(content); {
		if len(content)-offset < 8 {
			return false, errors.New("truncated WebP chunk")
		}
		chunkType := string(content[offset : offset+4])
		length := uint64(binary.LittleEndian.Uint32(content[offset+4 : offset+8]))
		paddedLength := length + length%2
		chunkSize := uint64(8) + paddedLength
		if chunkSize > uint64(len(content)-offset) {
			return false, errors.New("invalid WebP chunk size")
		}
		if chunkType == "ANIM" || chunkType == "ANMF" {
			return true, nil
		}
		if chunkType == "VP8X" {
			if length != 10 {
				return false, errors.New("invalid WebP extended header")
			}
			if content[offset+8]&0x02 != 0 {
				return true, nil
			}
		}
		offset += int(chunkSize)
	}
	return false, nil
}

func normalizeAvatarPixels(source image.Image, orientation int) image.Image {
	sourceBounds := source.Bounds()
	width, height := fitAvatarDimensions(sourceBounds.Dx(), sourceBounds.Dy())
	scaled := source
	if width != sourceBounds.Dx() || height != sourceBounds.Dy() {
		target := image.NewNRGBA(image.Rect(0, 0, width, height))
		xdraw.CatmullRom.Scale(target, target.Bounds(), source, sourceBounds, xdraw.Over, nil)
		scaled = target
	}
	return orientAvatarPixels(scaled, orientation)
}

func fitAvatarDimensions(width, height int) (int, int) {
	if width <= maxAvatarOutputSide && height <= maxAvatarOutputSide {
		return width, height
	}
	if width >= height {
		scaledHeight := (height*maxAvatarOutputSide + width/2) / width
		if scaledHeight < 1 {
			scaledHeight = 1
		}
		return maxAvatarOutputSide, scaledHeight
	}
	scaledWidth := (width*maxAvatarOutputSide + height/2) / height
	if scaledWidth < 1 {
		scaledWidth = 1
	}
	return scaledWidth, maxAvatarOutputSide
}

func hasAvatarTransparency(source image.Image) bool {
	bounds := source.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, alpha := source.At(x, y).RGBA()
			if alpha != 0xffff {
				return true
			}
		}
	}
	return false
}

func orientAvatarPixels(source image.Image, orientation int) image.Image {
	if orientation < 2 || orientation > 8 {
		return source
	}

	bounds := source.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	targetWidth, targetHeight := width, height
	if orientation >= 5 {
		targetWidth, targetHeight = height, width
	}
	target := image.NewNRGBA(image.Rect(0, 0, targetWidth, targetHeight))

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			targetX, targetY := orientedAvatarPoint(x, y, width, height, orientation)
			target.Set(targetX, targetY, source.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return target
}

func orientedAvatarPoint(x, y, width, height, orientation int) (int, int) {
	switch orientation {
	case 2:
		return width - 1 - x, y
	case 3:
		return width - 1 - x, height - 1 - y
	case 4:
		return x, height - 1 - y
	case 5:
		return y, x
	case 6:
		return height - 1 - y, x
	case 7:
		return height - 1 - y, width - 1 - x
	case 8:
		return y, width - 1 - x
	default:
		return x, y
	}
}

func avatarOrientation(content []byte, canonicalType CanonicalType) int {
	var tiff []byte
	switch canonicalType {
	case TypeJPEG:
		tiff = jpegEXIFPayload(content)
	case TypePNG:
		tiff = pngEXIFPayload(content)
	case TypeWebP:
		tiff = webpEXIFPayload(content)
	}
	if bytes.HasPrefix(tiff, []byte("Exif\x00\x00")) {
		tiff = tiff[6:]
	}
	if orientation, ok := tiffOrientation(tiff); ok {
		return orientation
	}
	return 1
}

func jpegEXIFPayload(content []byte) []byte {
	if len(content) < 4 || content[0] != 0xff || content[1] != 0xd8 {
		return nil
	}
	for offset := 2; offset+1 < len(content); {
		if content[offset] != 0xff {
			offset++
			continue
		}
		for offset < len(content) && content[offset] == 0xff {
			offset++
		}
		if offset >= len(content) {
			return nil
		}
		marker := content[offset]
		offset++
		if marker == 0xd9 || marker == 0xda {
			return nil
		}
		if marker == 0x01 || marker >= 0xd0 && marker <= 0xd7 {
			continue
		}
		if offset+2 > len(content) {
			return nil
		}
		length := int(binary.BigEndian.Uint16(content[offset : offset+2]))
		if length < 2 || length > len(content)-offset {
			return nil
		}
		payload := content[offset+2 : offset+length]
		if marker == 0xe1 && bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
			return payload
		}
		offset += length
	}
	return nil
}

func pngEXIFPayload(content []byte) []byte {
	if len(content) < 8 {
		return nil
	}
	for offset := 8; offset+12 <= len(content); {
		length := uint64(binary.BigEndian.Uint32(content[offset : offset+4]))
		chunkSize := uint64(12) + length
		if chunkSize > uint64(len(content)-offset) {
			return nil
		}
		chunkType := string(content[offset+4 : offset+8])
		if chunkType == "eXIf" {
			return content[offset+8 : offset+8+int(length)]
		}
		offset += int(chunkSize)
	}
	return nil
}

func webpEXIFPayload(content []byte) []byte {
	if len(content) < 12 {
		return nil
	}
	for offset := 12; offset+8 <= len(content); {
		length := uint64(binary.LittleEndian.Uint32(content[offset+4 : offset+8]))
		paddedLength := length + length%2
		chunkSize := uint64(8) + paddedLength
		if chunkSize > uint64(len(content)-offset) {
			return nil
		}
		if string(content[offset:offset+4]) == "EXIF" {
			return content[offset+8 : offset+8+int(length)]
		}
		offset += int(chunkSize)
	}
	return nil
}

func tiffOrientation(content []byte) (int, bool) {
	if len(content) < 8 {
		return 0, false
	}
	var order binary.ByteOrder
	switch string(content[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0, false
	}
	if order.Uint16(content[2:4]) != 42 {
		return 0, false
	}

	ifdOffset := uint64(order.Uint32(content[4:8]))
	if ifdOffset+2 > uint64(len(content)) {
		return 0, false
	}
	entryCount := uint64(order.Uint16(content[ifdOffset : ifdOffset+2]))
	entryOffset := ifdOffset + 2
	if entryCount > (uint64(len(content))-entryOffset)/12 {
		return 0, false
	}
	for index := uint64(0); index < entryCount; index++ {
		offset := entryOffset + index*12
		entry := content[offset : offset+12]
		if order.Uint16(entry[0:2]) != 0x0112 ||
			order.Uint16(entry[2:4]) != 3 ||
			order.Uint32(entry[4:8]) != 1 {
			continue
		}
		orientation := int(order.Uint16(entry[8:10]))
		if orientation >= 1 && orientation <= 8 {
			return orientation, true
		}
		return 0, false
	}
	return 0, false
}
