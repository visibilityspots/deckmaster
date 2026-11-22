package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// ImageCommandWidget is a button whose icon is chosen by a command: the first
// line the command prints is the path or icon name of the image to show. It
// accepts all options of a button; its icon is shown until the command first
// succeeds.
type ImageCommandWidget struct {
	*ButtonWidget

	command *commandRunner

	// the image currently shown, to only reload it when it changes
	current string
	modTime time.Time
}

// NewImageCommandWidget returns a new ImageCommandWidget.
func NewImageCommandWidget(bw *BaseWidget, opts WidgetConfig) (*ImageCommandWidget, error) {
	var command string
	r := configReader{config: opts.Config}
	r.read("command", &command)
	if r.err != nil {
		return nil, r.err
	}
	if command == "" {
		return nil, fmt.Errorf("no command configured")
	}

	button, err := NewButtonWidget(bw, opts)
	if err != nil {
		return nil, err
	}
	// after NewButtonWidget, which sets its own default interval
	bw.setInterval(msDuration(opts.Interval), time.Second)

	return &ImageCommandWidget{
		ButtonWidget: button,
		command:      newCommandRunner(fmt.Sprintf("key %d", bw.key), []string{command}),
	}, nil
}

// RequiresUpdate returns true when the widget wants to be repainted.
func (w *ImageCommandWidget) RequiresUpdate() bool {
	return w.command.Fresh() || w.ButtonWidget.RequiresUpdate()
}

// Update renders the widget.
func (w *ImageCommandWidget) Update() error {
	w.loadImage(firstLine(w.command.Outputs()[0]))
	return w.ButtonWidget.Update()
}

// loadImage shows the given image, unless it's already shown and unchanged
// on disk. A failing image keeps the previous one, and its error is only
// logged once.
func (w *ImageCommandWidget) loadImage(image string) {
	if image == "" {
		return
	}

	var modTime time.Time
	if !isIconName(image) {
		path, err := expandPath(w.base, image)
		if err == nil {
			if fi, err := os.Stat(path); err == nil {
				modTime = fi.ModTime()
			}
		}
	}
	if image == w.current && modTime.Equal(w.modTime) {
		return
	}

	changed := image != w.current
	w.current, w.modTime = image, modTime
	if err := w.LoadImage(image); err != nil && changed {
		fmt.Fprintf(os.Stderr, "key %d: can't load image %s: %s\n", w.key, image, err)
	}
}

// firstLine returns the first line of s, without surrounding whitespace.
func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}
