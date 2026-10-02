package media

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestSanitizeDecodesAndReencodesRasterImages(t *testing.T) {
	original := image.NewRGBA(image.Rect(0, 0, 3, 2))
	original.Set(1, 1, color.RGBA{R: 12, G: 34, B: 56, A: 255})
	var input bytes.Buffer
	if err := png.Encode(&input, original); err != nil {
		t.Fatal(err)
	}
	clean, mimeType, err := sanitize(input.Bytes())
	if err != nil || mimeType != "image/png" {
		t.Fatalf("sanitize = %q, %v", mimeType, err)
	}
	decoded, format, err := image.Decode(bytes.NewReader(clean))
	if err != nil || format != "png" || decoded.Bounds() != original.Bounds() {
		t.Fatalf("clean raster = %s %v %v", format, decoded, err)
	}
	got := color.RGBAModel.Convert(decoded.At(1, 1)).(color.RGBA)
	if got.R != 12 || got.G != 34 || got.B != 56 || got.A != 255 {
		t.Fatalf("pixel changed during safe re-encode: %#v", got)
	}
}

func TestSanitizeRejectsActiveAndMalformedFormats(t *testing.T) {
	for _, input := range [][]byte{[]byte("<svg><script>alert(1)</script></svg>"), []byte("not an image"), {}} {
		if _, _, err := sanitize(input); err == nil {
			t.Fatalf("accepted unsupported input %q", input)
		}
	}
}
