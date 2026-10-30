package imageutil

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const iconSize = 48

type iconEntry struct {
	modified time.Time
	size     int64
	data     string
}

// IconThumbnails keeps small, public image derivatives in memory only.
// It never fetches remote URLs or writes to the uploads directory.
type IconThumbnails struct {
	root  string
	mu    sync.Mutex
	cache map[string]iconEntry
}

func NewIconThumbnails(root string) *IconThumbnails {
	return &IconThumbnails{root: root, cache: make(map[string]iconEntry)}
}

// DataURL returns an at-most-48px PNG, or a small SVG, for a local upload.
func (icons *IconThumbnails) DataURL(source string) string {
	if !strings.HasPrefix(source, "/uploads/") {
		return ""
	}
	relative := strings.TrimPrefix(source, "/uploads/")
	if !filepath.IsLocal(relative) {
		return ""
	}
	root, err := os.OpenRoot(icons.root)
	if err != nil {
		return ""
	}
	defer root.Close()
	file, err := root.Open(relative)
	if err != nil {
		return ""
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return ""
	}
	icons.mu.Lock()
	cached, exists := icons.cache[source]
	icons.mu.Unlock()
	if exists && cached.modified.Equal(info.ModTime()) && cached.size == info.Size() {
		return cached.data
	}

	var output bytes.Buffer
	mime := "image/png"
	if strings.EqualFold(filepath.Ext(relative), ".svg") {
		// Small vectors already scale without a large raster download.
		if info.Size() > 16<<10 {
			return ""
		}
		if _, err := io.Copy(&output, io.LimitReader(file, 16<<10)); err != nil {
			return ""
		}
		mime = "image/svg+xml"
	} else {
		config, _, err := image.DecodeConfig(io.LimitReader(file, 16<<20))
		if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 4096 || config.Height > 4096 {
			return ""
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return ""
		}
		original, _, err := image.Decode(io.LimitReader(file, 16<<20))
		if err != nil {
			return ""
		}
		width, height := config.Width, config.Height
		if width > iconSize || height > iconSize {
			if width >= height {
				height = max(1, height*iconSize/width)
				width = iconSize
			} else {
				width = max(1, width*iconSize/height)
				height = iconSize
			}
		}
		thumbnail := image.NewNRGBA(image.Rect(0, 0, width, height))
		draw.CatmullRom.Scale(thumbnail, thumbnail.Bounds(), original, original.Bounds(), draw.Src, nil)
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		if err := encoder.Encode(&output, thumbnail); err != nil {
			return ""
		}
	}
	data := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(output.Bytes())
	icons.mu.Lock()
	if len(icons.cache) >= 128 {
		clear(icons.cache)
	}
	icons.cache[source] = iconEntry{modified: info.ModTime(), size: info.Size(), data: data}
	icons.mu.Unlock()
	return data
}
