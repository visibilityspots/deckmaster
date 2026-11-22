package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeSquare writes a PNG filled with a single color.
func writeSquare(t *testing.T, path string, c color.Color) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

// centerColor returns the color in the middle of an image.
func centerColor(img image.Image) color.RGBA {
	b := img.Bounds()
	r, g, bl, a := img.At(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2).RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), uint8(a >> 8)}
}

func TestImageCommandWidget(t *testing.T) {
	dir := t.TempDir()
	red, blue := filepath.Join(dir, "red.png"), filepath.Join(dir, "blue.png")
	writeSquare(t, red, color.RGBA{255, 0, 0, 255})
	writeSquare(t, blue, color.RGBA{0, 0, 255, 255})
	state := filepath.Join(dir, "state")
	if err := os.WriteFile(state, []byte(red), 0o600); err != nil {
		t.Fatal(err)
	}

	w, out := renderWidget(t, xlDevice(), displayKey, WidgetConfig{ID: "imageCommand", Config: map[string]interface{}{
		"command": "cat " + state + "; echo; echo ignored second line",
	}})
	iw := w.(*ImageCommandWidget)

	// the first update has no output yet
	if n := litPixels(out.image(t, "key 0")); n != 0 {
		t.Errorf("first update drew %d pixels, want none", n)
	}

	update := func() {
		t.Helper()
		waitForFresh(t, iw.command)
		if err := w.Update(); err != nil {
			t.Fatal(err)
		}
	}
	update()
	if c := centerColor(out.image(t, "key 0")); c.R != 255 || c.B != 0 {
		t.Errorf("shows %v, want red", c)
	}

	// the command switches to another image
	if err := os.WriteFile(state, []byte(blue), 0o600); err != nil {
		t.Fatal(err)
	}
	w.Update() // starts the command again
	update()
	if c := centerColor(out.image(t, "key 0")); c.B != 255 || c.R != 0 {
		t.Errorf("shows %v, want blue", c)
	}

	// same path, new content: reloaded because the file changed
	writeSquare(t, blue, color.RGBA{0, 255, 0, 255})
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(blue, later, later); err != nil {
		t.Fatal(err)
	}
	w.Update()
	update()
	if c := centerColor(out.image(t, "key 0")); c.G != 255 {
		t.Errorf("shows %v, want green", c)
	}

	// a missing image keeps the last one
	if err := os.WriteFile(state, []byte(filepath.Join(dir, "missing.png")), 0o600); err != nil {
		t.Fatal(err)
	}
	w.Update()
	update()
	if c := centerColor(out.image(t, "key 0")); c.G != 255 {
		t.Errorf("shows %v after a missing image, want the last one (green)", c)
	}
}

func TestImageCommandWidgetNeedsCommand(t *testing.T) {
	bw := NewBaseWidget(xlDevice(), ".", 0, nil, nil, nil)
	if _, err := newWidget(bw, WidgetConfig{ID: "imageCommand"}); err == nil {
		t.Error("imageCommand widget without a command: no error")
	}
}

func TestFirstLine(t *testing.T) {
	for in, want := range map[string]string{
		"":                 "",
		"/a/b.png":         "/a/b.png",
		"  /a/b.png  \nxx": "/a/b.png",
		"name\n":           "name",
	} {
		if got := firstLine(in); got != want {
			t.Errorf("firstLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestImageCommandWidgetRetriesBrokenImage(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "badge.png")
	// half-written: not a PNG yet
	if err := os.WriteFile(img, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	w, out := renderWidget(t, xlDevice(), displayKey, WidgetConfig{ID: "imageCommand", Config: map[string]interface{}{
		"command": "echo " + img,
	}})
	iw := w.(*ImageCommandWidget)
	waitForFresh(t, iw.command)
	if err := w.Update(); err != nil {
		t.Fatal(err)
	}

	// finished writing, with the same modification time as the broken file
	fi, err := os.Stat(img)
	if err != nil {
		t.Fatal(err)
	}
	writeSquare(t, img, color.RGBA{255, 0, 0, 255})
	if err := os.Chtimes(img, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	w.Update()
	waitForFresh(t, iw.command)
	if err := w.Update(); err != nil {
		t.Fatal(err)
	}
	if c := centerColor(out.image(t, "key 0")); c.R != 255 {
		t.Errorf("shows %v, want the finished red image", c)
	}
}
