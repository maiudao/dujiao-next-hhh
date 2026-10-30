package imageutil

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeIcon(t *testing.T, filename string, width, height int, shade color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, shade)
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, output.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestIconThumbnailSizeCacheAndOriginal(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "icon.png")
	original := writeIcon(t, filename, 1024, 512, color.NRGBA{R: 80, A: 200})
	icons := NewIconThumbnails(root)
	data := icons.DataURL("/uploads/icon.png")
	if !strings.HasPrefix(data, "data:image/png;base64,") {
		t.Fatalf("missing PNG: %q", data)
	}
	encoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(data, "data:image/png;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(encoded))
	if err != nil || config.Width != 48 || config.Height != 24 {
		t.Fatalf("thumbnail: %+v %v", config, err)
	}
	if len(encoded) >= len(original) {
		t.Fatalf("not compressed: %d >= %d", len(encoded), len(original))
	}
	current, err := os.ReadFile(filename)
	if err != nil || sha256.Sum256(current) != sha256.Sum256(original) {
		t.Fatal("original changed")
	}
	if icons.DataURL("/uploads/icon.png") != data || len(icons.cache) != 1 {
		t.Fatal("cache miss")
	}
	writeIcon(t, filename, 1024, 512, color.NRGBA{B: 180, A: 255})
	stamp := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(filename, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if icons.DataURL("/uploads/icon.png") == data {
		t.Fatal("stale thumbnail after source change")
	}
	t.Logf("original=%d bytes thumbnail=%d bytes, 48x24, source unchanged", len(original), len(encoded))
}

func TestIconThumbnailRejectsUnsafeOrOversizedSources(t *testing.T) {
	root := t.TempDir()
	writeIcon(t, filepath.Join(root, "large.png"), 4097, 1, color.NRGBA{A: 255})
	if err := os.WriteFile(filepath.Join(root, "invalid.png"), []byte("invalid image"), 0600); err != nil {
		t.Fatal(err)
	}
	icons := NewIconThumbnails(root)
	for _, source := range []string{"", "https://example.invalid/icon.png", "/uploads/../../secret.png", "/uploads/missing.png", "/uploads/invalid.png", "/uploads/large.png"} {
		if icons.DataURL(source) != "" {
			t.Fatalf("accepted %q", source)
		}
	}
	outside := filepath.Join(t.TempDir(), "private.png")
	writeIcon(t, outside, 10, 10, color.NRGBA{A: 255})
	if err := os.Symlink(outside, filepath.Join(root, "escape.png")); err == nil {
		if icons.DataURL("/uploads/escape.png") != "" {
			t.Fatal("followed symlink outside uploads")
		}
	} else {
		t.Log("symlink creation unavailable; traversal cases passed")
	}
}

func TestIconThumbnailSmallImagesSVGAndCacheLimit(t *testing.T) {
	root := t.TempDir()
	writeIcon(t, filepath.Join(root, "small.png"), 12, 8, color.NRGBA{A: 255})
	icons := NewIconThumbnails(root)
	data := icons.DataURL("/uploads/small.png")
	encoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(data, "data:image/png;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(encoded))
	if err != nil || config.Width != 12 || config.Height != 8 {
		t.Fatal("small icon enlarged")
	}
	if err := os.WriteFile(filepath.Join(root, "icon.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"/>`), 0600); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(icons.DataURL("/uploads/icon.svg"), "data:image/svg+xml;base64,") {
		t.Fatal("small SVG missing")
	}
	if err := os.WriteFile(filepath.Join(root, "big.svg"), bytes.Repeat([]byte(" "), 17<<10), 0600); err != nil {
		t.Fatal(err)
	}
	if icons.DataURL("/uploads/big.svg") != "" {
		t.Fatal("large SVG accepted")
	}
	clear(icons.cache)
	for i := 0; i < 128; i++ {
		icons.cache[time.Unix(int64(i), 0).String()] = iconEntry{}
	}
	icons.DataURL("/uploads/small.png")
	if len(icons.cache) > 128 {
		t.Fatal("unbounded cache")
	}
}
