package uploadsecurity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"testing"

	"admin/service/uploadsecurity"
)

func TestAvatarValidatorReturnsNormalizedTrustedUploadResult(t *testing.T) {
	content := encodeAlphaPNG(t, false)
	validator := uploadsecurity.NewAvatarValidator()

	result, err := validator.Validate(context.Background(), uploadsecurity.Input{
		Purpose:      uploadsecurity.PurposeAvatar,
		FileName:     "portrait.png",
		DeclaredMIME: "image/png",
		Size:         int64(len(content)),
		MaxBytes:     2 << 20,
		Reader:       bytes.NewReader(content),
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if result.Purpose != uploadsecurity.PurposeAvatar ||
		result.FileName != "portrait.jpg" ||
		result.CanonicalType != uploadsecurity.TypeJPEG ||
		result.CanonicalExtension != ".jpg" ||
		result.CanonicalMIME != "image/jpeg" ||
		result.DetectedMIME != "image/png" ||
		result.PolicyVersion != uploadsecurity.PolicyVersionV1 {
		t.Fatalf("Validate() result = %#v, want normalized trusted JPEG metadata", result)
	}
	normalized, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatalf("read normalized avatar: %v", err)
	}
	if int64(len(normalized)) != result.Size {
		t.Fatalf("normalized size = %d, result size = %d", len(normalized), result.Size)
	}
	if _, format, err := image.Decode(bytes.NewReader(normalized)); err != nil || format != "jpeg" {
		t.Fatalf("normalized avatar format = %q, error = %v, want jpeg", format, err)
	}
}

func TestAvatarResultDigestMatchesNormalizedOutputNotOriginalInput(t *testing.T) {
	content := encodeAlphaPNG(t, false)
	result, err := uploadsecurity.NewAvatarValidator().Validate(
		context.Background(),
		uploadsecurity.Input{
			Purpose:      uploadsecurity.PurposeAvatar,
			FileName:     "portrait.png",
			DeclaredMIME: "image/png",
			Size:         int64(len(content)),
			MaxBytes:     2 << 20,
			Reader:       bytes.NewReader(content),
		},
	)
	if err != nil {
		t.Fatalf("validate avatar: %v", err)
	}

	normalized, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatalf("read normalized avatar: %v", err)
	}
	outputSum := sha256.Sum256(normalized)
	inputSum := sha256.Sum256(content)
	wantOutputDigest := hex.EncodeToString(outputSum[:])
	inputDigest := hex.EncodeToString(inputSum[:])
	if result.ContentSHA256 != wantOutputDigest {
		t.Fatalf("normalized content sha256: got %q, want %q", result.ContentSHA256, wantOutputDigest)
	}
	if result.ContentSHA256 == inputDigest {
		t.Fatalf("avatar digest unexpectedly matches original input digest %q", inputDigest)
	}
}

func TestProbeAvatarHeaderAcceptsDimensionBoundariesWithoutFullDecode(t *testing.T) {
	tests := []struct {
		name          string
		canonicalType uploadsecurity.CanonicalType
		width         int
		height        int
		content       func(t *testing.T, width, height int) []byte
	}{
		{"jpeg side boundary", uploadsecurity.TypeJPEG, 8192, 1, jpegHeaderWithDimensions},
		{"jpeg pixel boundary", uploadsecurity.TypeJPEG, 8000, 5000, jpegHeaderWithDimensions},
		{"png side boundary", uploadsecurity.TypePNG, 8192, 1, pngHeaderWithDimensions},
		{"png pixel boundary", uploadsecurity.TypePNG, 8000, 5000, pngHeaderWithDimensions},
		{"webp side boundary", uploadsecurity.TypeWebP, 8192, 1, webpHeaderWithDimensions},
		{"webp pixel boundary", uploadsecurity.TypeWebP, 8000, 5000, webpHeaderWithDimensions},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probe, err := uploadsecurity.ProbeAvatarHeader(tt.content(t, tt.width, tt.height))
			if err != nil {
				t.Fatalf("probe avatar header: %v", err)
			}
			if probe.CanonicalType != tt.canonicalType {
				t.Fatalf("canonical type: got %q, want %q", probe.CanonicalType, tt.canonicalType)
			}
			if probe.Width != tt.width || probe.Height != tt.height {
				t.Fatalf("dimensions: got %dx%d, want %dx%d", probe.Width, probe.Height, tt.width, tt.height)
			}
		})
	}
}

func TestProbeAvatarHeaderRejectsDimensionLimitsBeforeFullDecode(t *testing.T) {
	tests := []struct {
		name    string
		width   int
		height  int
		content func(t *testing.T, width, height int) []byte
	}{
		{"jpeg side over limit", 8193, 1, jpegHeaderWithDimensions},
		{"jpeg pixels over limit", 8000, 5001, jpegHeaderWithDimensions},
		{"png side over limit", 8193, 1, pngHeaderWithDimensions},
		{"png pixels over limit", 8000, 5001, pngHeaderWithDimensions},
		{"webp side over limit", 8193, 1, webpHeaderWithDimensions},
		{"webp pixels over limit", 8000, 5001, webpHeaderWithDimensions},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := uploadsecurity.ProbeAvatarHeader(tt.content(t, tt.width, tt.height))
			code, ok := uploadsecurity.CodeOf(err)
			if !ok || code != uploadsecurity.CodeImageDimensionLimit {
				t.Fatalf("dimension error: got %q, classified=%v", code, ok)
			}
		})
	}
}

func TestNormalizeAvatarFullyDecodesSupportedStaticImages(t *testing.T) {
	tests := []struct {
		name          string
		canonicalType uploadsecurity.CanonicalType
		content       []byte
	}{
		{"jpeg", uploadsecurity.TypeJPEG, encodeTestJPEG(t)},
		{"png", uploadsecurity.TypePNG, encodeTestPNG(t)},
		{"webp", uploadsecurity.TypeWebP, decodeTestWebP(t)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := uploadsecurity.NormalizeAvatar(tt.content, tt.canonicalType, 2<<20)
			if err != nil {
				t.Fatalf("normalize avatar: %v", err)
			}
			if result.InputType != tt.canonicalType {
				t.Fatalf("input type: got %q, want %q", result.InputType, tt.canonicalType)
			}
		})
	}
}

func TestNormalizeAvatarRejectsCorruptMismatchedAndAnimatedImages(t *testing.T) {
	tests := []struct {
		name          string
		canonicalType uploadsecurity.CanonicalType
		content       []byte
		wantCode      uploadsecurity.Code
	}{
		{
			name:          "corrupt JPEG after valid header",
			canonicalType: uploadsecurity.TypeJPEG,
			content:       jpegHeaderWithDimensions(t, 2, 2),
			wantCode:      uploadsecurity.CodeImageDecodeInvalid,
		},
		{
			name:          "declared JPEG contains PNG",
			canonicalType: uploadsecurity.TypeJPEG,
			content:       encodeTestPNG(t),
			wantCode:      uploadsecurity.CodeFileTypeMismatch,
		},
		{
			name:          "animated PNG",
			canonicalType: uploadsecurity.TypePNG,
			content:       encodeTestAPNG(t),
			wantCode:      uploadsecurity.CodeImageDecodeInvalid,
		},
		{
			name:          "animated WebP",
			canonicalType: uploadsecurity.TypeWebP,
			content:       encodeTestAnimatedWebP(t),
			wantCode:      uploadsecurity.CodeImageDecodeInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := uploadsecurity.NormalizeAvatar(tt.content, tt.canonicalType, 2<<20)
			code, ok := uploadsecurity.CodeOf(err)
			if !ok || code != tt.wantCode {
				t.Fatalf("normalize error: got %q, classified=%v, want %q", code, ok, tt.wantCode)
			}
		})
	}
}

func TestNormalizeAvatarScalesProportionallyWithoutUpscaling(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		height     int
		wantWidth  int
		wantHeight int
	}{
		{"landscape reaches width limit", 2048, 1024, 1024, 512},
		{"portrait reaches height limit", 512, 2048, 256, 1024},
		{"small image is not enlarged", 320, 200, 320, 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := uploadsecurity.NormalizeAvatar(
				encodeSolidPNG(t, tt.width, tt.height),
				uploadsecurity.TypePNG,
				10<<20,
			)
			if err != nil {
				t.Fatalf("normalize avatar: %v", err)
			}
			if result.Width != tt.wantWidth || result.Height != tt.wantHeight {
				t.Fatalf(
					"normalized dimensions: got %dx%d, want %dx%d",
					result.Width,
					result.Height,
					tt.wantWidth,
					tt.wantHeight,
				)
			}
		})
	}
}

func TestNormalizeAvatarAppliesEXIFOrientationBeforeReportingDimensions(t *testing.T) {
	content := encodeJPEGWithEXIFOrientation(t, 2, 3, 6)
	result, err := uploadsecurity.NormalizeAvatar(content, uploadsecurity.TypeJPEG, 2<<20)
	if err != nil {
		t.Fatalf("normalize avatar: %v", err)
	}
	if result.Width != 3 || result.Height != 2 {
		t.Fatalf("oriented dimensions: got %dx%d, want 3x2", result.Width, result.Height)
	}
}

func TestNormalizeAvatarReencodesOpaqueAndTransparentPixelsToTrustedFormats(t *testing.T) {
	tests := []struct {
		name          string
		content       []byte
		wantType      uploadsecurity.CanonicalType
		wantExtension string
		wantMIME      string
		wantFormat    string
	}{
		{
			name:          "opaque PNG becomes JPEG",
			content:       encodeAlphaPNG(t, false),
			wantType:      uploadsecurity.TypeJPEG,
			wantExtension: ".jpg",
			wantMIME:      "image/jpeg",
			wantFormat:    "jpeg",
		},
		{
			name:          "transparent PNG remains PNG",
			content:       encodeAlphaPNG(t, true),
			wantType:      uploadsecurity.TypePNG,
			wantExtension: ".png",
			wantMIME:      "image/png",
			wantFormat:    "png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := uploadsecurity.NormalizeAvatar(tt.content, uploadsecurity.TypePNG, 2<<20)
			if err != nil {
				t.Fatalf("normalize avatar: %v", err)
			}
			if result.CanonicalType != tt.wantType ||
				result.Extension != tt.wantExtension ||
				result.MIME != tt.wantMIME {
				t.Fatalf("trusted output metadata: got %+v", result)
			}
			_, format, err := image.Decode(bytes.NewReader(result.Data))
			if err != nil {
				t.Fatalf("decode normalized output: %v", err)
			}
			if format != tt.wantFormat {
				t.Fatalf("output format: got %q, want %q", format, tt.wantFormat)
			}
		})
	}
}

func TestNormalizeAvatarRemovesNonPixelMetadataByReencoding(t *testing.T) {
	tests := []struct {
		name          string
		content       []byte
		canonicalType uploadsecurity.CanonicalType
		secret        []byte
	}{
		{
			name:          "JPEG EXIF",
			content:       encodeJPEGWithEXIFOrientation(t, 2, 3, 6),
			canonicalType: uploadsecurity.TypeJPEG,
			secret:        []byte("Exif\x00\x00"),
		},
		{
			name:          "PNG text metadata",
			content:       encodeTransparentPNGWithText(t, "secret-device-location"),
			canonicalType: uploadsecurity.TypePNG,
			secret:        []byte("secret-device-location"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := uploadsecurity.NormalizeAvatar(tt.content, tt.canonicalType, 2<<20)
			if err != nil {
				t.Fatalf("normalize avatar: %v", err)
			}
			if bytes.Contains(result.Data, tt.secret) {
				t.Fatalf("normalized output retained metadata %q", tt.secret)
			}
		})
	}
}

func TestNormalizeAvatarEnforcesReencodedOutputSizeBoundary(t *testing.T) {
	content := encodeAlphaPNG(t, true)
	baseline, err := uploadsecurity.NormalizeAvatar(content, uploadsecurity.TypePNG, 2<<20)
	if err != nil {
		t.Fatalf("build baseline output: %v", err)
	}
	if len(baseline.Data) < 2 {
		t.Fatalf("unexpected baseline output size: %d", len(baseline.Data))
	}

	exact, err := uploadsecurity.NormalizeAvatar(content, uploadsecurity.TypePNG, int64(len(baseline.Data)))
	if err != nil {
		t.Fatalf("exact output boundary should be accepted: %v", err)
	}
	if len(exact.Data) != len(baseline.Data) {
		t.Fatalf("exact output size: got %d, want %d", len(exact.Data), len(baseline.Data))
	}

	_, err = uploadsecurity.NormalizeAvatar(content, uploadsecurity.TypePNG, int64(len(baseline.Data)-1))
	code, ok := uploadsecurity.CodeOf(err)
	if !ok || code != uploadsecurity.CodeFileTooLarge {
		t.Fatalf("oversized normalized output: got %q, classified=%v", code, ok)
	}
}

func TestNormalizeAvatarConvertsWebPToTrustedJPEGOrPNGOutput(t *testing.T) {
	result, err := uploadsecurity.NormalizeAvatar(decodeTestWebP(t), uploadsecurity.TypeWebP, 2<<20)
	if err != nil {
		t.Fatalf("normalize WebP avatar: %v", err)
	}
	if result.InputType != uploadsecurity.TypeWebP {
		t.Fatalf("input type: got %q", result.InputType)
	}
	if result.CanonicalType != uploadsecurity.TypeJPEG && result.CanonicalType != uploadsecurity.TypePNG {
		t.Fatalf("WebP output type is not trusted: %q", result.CanonicalType)
	}
	_, format, err := image.Decode(bytes.NewReader(result.Data))
	if err != nil {
		t.Fatalf("decode normalized WebP output: %v", err)
	}
	if format == "webp" || format != string(result.CanonicalType) {
		t.Fatalf("output bytes format %q does not match %q", format, result.CanonicalType)
	}
}

func TestNormalizeAvatarAppliesOrientationToPixelPositions(t *testing.T) {
	content := encodeOrientedTransparentPNG(t, 6)
	result, err := uploadsecurity.NormalizeAvatar(content, uploadsecurity.TypePNG, 2<<20)
	if err != nil {
		t.Fatalf("normalize oriented PNG: %v", err)
	}
	if result.CanonicalType != uploadsecurity.TypePNG || result.Width != 3 || result.Height != 2 {
		t.Fatalf("oriented output: got type=%q size=%dx%d", result.CanonicalType, result.Width, result.Height)
	}

	decoded, _, err := image.Decode(bytes.NewReader(result.Data))
	if err != nil {
		t.Fatalf("decode oriented output: %v", err)
	}
	assertNRGBAAt(t, decoded, 0, 0, color.NRGBA{B: 255, A: 255})
	assertNRGBAAt(t, decoded, 2, 0, color.NRGBA{R: 255, A: 255})
	assertNRGBAAt(t, decoded, 0, 1, color.NRGBA{R: 255, B: 255, A: 255})
	assertNRGBAAt(t, decoded, 2, 1, color.NRGBA{R: 255, G: 255, A: 255})
}

func jpegHeaderWithDimensions(t *testing.T, width, height int) []byte {
	t.Helper()
	content := append([]byte(nil), encodeTestJPEG(t)...)
	for index := 0; index+8 < len(content); index++ {
		if content[index] != 0xff || content[index+1] < 0xc0 || content[index+1] > 0xc3 {
			continue
		}
		binary.BigEndian.PutUint16(content[index+5:index+7], uint16(height))
		binary.BigEndian.PutUint16(content[index+7:index+9], uint16(width))
		for scan := index + 9; scan+3 < len(content); scan++ {
			if content[scan] != 0xff || content[scan+1] != 0xda {
				continue
			}
			segmentLength := int(binary.BigEndian.Uint16(content[scan+2 : scan+4]))
			return content[:scan+2+segmentLength]
		}
		t.Fatal("JPEG fixture has no start-of-scan marker")
	}
	t.Fatal("JPEG fixture has no start-of-frame marker")
	return nil
}

func pngHeaderWithDimensions(t *testing.T, width, height int) []byte {
	t.Helper()
	content := append([]byte(nil), encodeTestPNG(t)...)
	if len(content) < 33 || string(content[12:16]) != "IHDR" {
		t.Fatal("PNG fixture has no IHDR chunk")
	}
	binary.BigEndian.PutUint32(content[16:20], uint32(width))
	binary.BigEndian.PutUint32(content[20:24], uint32(height))
	binary.BigEndian.PutUint32(content[29:33], crc32.ChecksumIEEE(content[12:29]))
	return content[:33]
}

func webpHeaderWithDimensions(t *testing.T, width, height int) []byte {
	t.Helper()
	content := append([]byte(nil), decodeTestWebP(t)...)
	if len(content) < 25 || string(content[12:16]) != "VP8L" || content[20] != 0x2f {
		t.Fatal("WebP fixture is not a VP8L image")
	}
	packed := uint32(width-1) | uint32(height-1)<<14 | uint32(content[24]&0x10)<<24
	content[21] = byte(packed)
	content[22] = byte(packed >> 8)
	content[23] = byte(packed >> 16)
	content[24] = byte(packed >> 24)
	return content[:25]
}

func encodeTestAPNG(t *testing.T) []byte {
	t.Helper()
	content := encodeTestPNG(t)
	if len(content) < 33 || string(content[12:16]) != "IHDR" {
		t.Fatal("PNG fixture has no IHDR chunk")
	}

	animationControl := make([]byte, 8)
	binary.BigEndian.PutUint32(animationControl[0:4], 2)
	chunk := pngChunk("acTL", animationControl)
	result := make([]byte, 0, len(content)+len(chunk))
	result = append(result, content[:33]...)
	result = append(result, chunk...)
	result = append(result, content[33:]...)
	return result
}

func pngChunk(chunkType string, data []byte) []byte {
	chunk := make([]byte, 12+len(data))
	binary.BigEndian.PutUint32(chunk[0:4], uint32(len(data)))
	copy(chunk[4:8], chunkType)
	copy(chunk[8:8+len(data)], data)
	binary.BigEndian.PutUint32(chunk[8+len(data):], crc32.ChecksumIEEE(chunk[4:8+len(data)]))
	return chunk
}

func encodeTestAnimatedWebP(t *testing.T) []byte {
	t.Helper()
	static := decodeTestWebP(t)
	if len(static) < 12 || string(static[0:4]) != "RIFF" || string(static[8:12]) != "WEBP" {
		t.Fatal("invalid static WebP fixture")
	}

	extended := make([]byte, 18)
	copy(extended[0:4], "VP8X")
	binary.LittleEndian.PutUint32(extended[4:8], 10)
	extended[8] = 0x02
	extended[12] = 1
	extended[15] = 1

	result := make([]byte, 0, len(static)+len(extended))
	result = append(result, static[:12]...)
	result = append(result, extended...)
	result = append(result, static[12:]...)
	binary.LittleEndian.PutUint32(result[4:8], uint32(len(result)-8))
	return result
}

func encodeSolidPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewNRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatalf("encode PNG: %v", err)
	}
	return buffer.Bytes()
}

func encodeJPEGWithEXIFOrientation(t *testing.T, width, height, orientation int) []byte {
	t.Helper()
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, width, height)), nil); err != nil {
		t.Fatalf("encode JPEG: %v", err)
	}
	content := encoded.Bytes()

	tiff := make([]byte, 26)
	copy(tiff[0:2], "II")
	binary.LittleEndian.PutUint16(tiff[2:4], 42)
	binary.LittleEndian.PutUint32(tiff[4:8], 8)
	binary.LittleEndian.PutUint16(tiff[8:10], 1)
	binary.LittleEndian.PutUint16(tiff[10:12], 0x0112)
	binary.LittleEndian.PutUint16(tiff[12:14], 3)
	binary.LittleEndian.PutUint32(tiff[14:18], 1)
	binary.LittleEndian.PutUint16(tiff[18:20], uint16(orientation))

	payload := append([]byte("Exif\x00\x00"), tiff...)
	app1 := make([]byte, 4+len(payload))
	app1[0] = 0xff
	app1[1] = 0xe1
	binary.BigEndian.PutUint16(app1[2:4], uint16(len(payload)+2))
	copy(app1[4:], payload)

	result := make([]byte, 0, len(content)+len(app1))
	result = append(result, content[:2]...)
	result = append(result, app1...)
	result = append(result, content[2:]...)
	return result
}

func encodeAlphaPNG(t *testing.T, transparent bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(50 + x*80), G: uint8(50 + y*80), B: 180, A: 255})
		}
	}
	if transparent {
		img.SetNRGBA(1, 1, color.NRGBA{R: 200, G: 100, B: 50, A: 0})
	}

	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("encode alpha PNG: %v", err)
	}
	return buffer.Bytes()
}

func encodeTransparentPNGWithText(t *testing.T, secret string) []byte {
	t.Helper()
	content := encodeAlphaPNG(t, true)
	chunk := pngChunk("tEXt", append([]byte("Comment\x00"), []byte(secret)...))
	result := make([]byte, 0, len(content)+len(chunk))
	result = append(result, content[:33]...)
	result = append(result, chunk...)
	result = append(result, content[33:]...)
	return result
}

func encodeOrientedTransparentPNG(t *testing.T, orientation int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 3))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(0, 1, color.NRGBA{G: 255, A: 255})
	img.SetNRGBA(0, 2, color.NRGBA{B: 255, A: 255})
	img.SetNRGBA(1, 0, color.NRGBA{R: 255, G: 255, A: 255})
	img.SetNRGBA(1, 1, color.NRGBA{})
	img.SetNRGBA(1, 2, color.NRGBA{R: 255, B: 255, A: 255})

	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("encode oriented PNG: %v", err)
	}
	content := buffer.Bytes()

	tiff := make([]byte, 26)
	copy(tiff[0:2], "II")
	binary.LittleEndian.PutUint16(tiff[2:4], 42)
	binary.LittleEndian.PutUint32(tiff[4:8], 8)
	binary.LittleEndian.PutUint16(tiff[8:10], 1)
	binary.LittleEndian.PutUint16(tiff[10:12], 0x0112)
	binary.LittleEndian.PutUint16(tiff[12:14], 3)
	binary.LittleEndian.PutUint32(tiff[14:18], 1)
	binary.LittleEndian.PutUint16(tiff[18:20], uint16(orientation))

	chunk := pngChunk("eXIf", tiff)
	result := make([]byte, 0, len(content)+len(chunk))
	result = append(result, content[:33]...)
	result = append(result, chunk...)
	result = append(result, content[33:]...)
	return result
}

func assertNRGBAAt(t *testing.T, img image.Image, x, y int, want color.NRGBA) {
	t.Helper()
	got := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
	if got != want {
		t.Fatalf("pixel (%d,%d): got %+v, want %+v", x, y, got, want)
	}
}
