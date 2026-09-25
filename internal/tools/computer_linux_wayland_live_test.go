//go:build linux

package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const waylandProbe = `
import sys
import gi
gi.require_version("Gtk", "3.0")
gi.require_version("Gdk", "3.0")
from gi.repository import Gdk, Gtk

mode = sys.argv[1]
css = Gtk.CssProvider()
css.load_from_data(b"window { background-color: #123456; } button { background-color: #f0c040; }")
Gtk.StyleContext.add_provider_for_screen(Gdk.Screen.get_default(), css, Gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
window = Gtk.Window(title="SlimeBot Wayland " + mode)
window.set_default_size(500, 260)
window.fullscreen()
window.connect("destroy", Gtk.main_quit)
box = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=20)
box.pack_start(Gtk.Label(label="SlimeBot native Wayland probe"), False, False, 40)
if mode == "entry":
    entry = Gtk.Entry()
    entry.connect("activate", lambda widget: (print(widget.get_text(), flush=True), Gtk.main_quit()))
    entry.connect("focus-in-event", lambda widget, event: (print("entry focused", file=sys.stderr, flush=True), False)[1])
    entry.connect("key-press-event", lambda widget, event: (print("key", event.keyval, file=sys.stderr, flush=True), False)[1])
    box.pack_start(entry, True, True, 20)
else:
    button = Gtk.Button(label="WAYLAND CLICK")
    button.connect("clicked", lambda widget: (print("clicked", flush=True), Gtk.main_quit()))
    box.pack_start(button, True, True, 20)
window.add(box)
window.show_all()
if mode == "entry":
    entry.grab_focus()
Gtk.main()
`

func imageHasContrast(img image.Image) bool {
	bounds := img.Bounds()
	if bounds.Dx() < 400 || bounds.Dy() < 200 {
		return false
	}
	minLight, maxLight := uint32(3*65535), uint32(0)
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 20 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 20 {
			r, g, b, _ := img.At(x, y).RGBA()
			light := r + g + b
			if light < minLight {
				minLight = light
			}
			if light > maxLight {
				maxLight = light
			}
		}
	}
	return maxLight-minLight >= 3*20*257
}

func TestComputerLinuxWaylandLive(t *testing.T) {
	if os.Getenv("SLIMEBOT_COMPUTER_WAYLAND_LIVE_TEST") != "1" {
		t.Skip("set SLIMEBOT_COMPUTER_WAYLAND_LIVE_TEST=1 inside a Wayland session")
	}
	if os.Getenv("WAYLAND_DISPLAY") == "" || os.Getenv("DISPLAY") != "" {
		t.Fatal("a native Wayland session without DISPLAY is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	tool := &computerTool{snapshots: make(map[string]*computerSnapshot)}

	startProbe := func(mode string) (<-chan error, *bytes.Buffer, *bytes.Buffer) {
		cmd := exec.CommandContext(ctx, "python3", "-c", waylandProbe, mode)
		cmd.Env = append(os.Environ(), "GDK_BACKEND=wayland")
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		cmd.Stdout, cmd.Stderr = stdout, stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		return done, stdout, stderr
	}

	screenshot := func() (string, image.Image) {
		deadline := time.Now().Add(10 * time.Second)
		var result *ExecuteResult
		var err error
		for time.Now().Before(deadline) && ctx.Err() == nil {
			result, err = tool.Execute(ctx, "observe", map[string]any{"mode": "screenshot"})
			if err == nil && strings.HasPrefix(result.ImageURL, "data:image/jpeg;base64,") {
				data, decodeErr := base64.StdEncoding.DecodeString(strings.TrimPrefix(result.ImageURL, "data:image/jpeg;base64,"))
				if decodeErr == nil {
					img, imageErr := jpeg.Decode(bytes.NewReader(data))
					if imageErr == nil && imageHasContrast(img) {
						var snapshot struct {
							ID     string `json:"snapshot_id"`
							Source string `json:"source"`
						}
						if json.Unmarshal([]byte(result.Output), &snapshot) == nil && snapshot.ID != "" && snapshot.Source == "screenshot" {
							if dir := os.Getenv("RUNNER_TEMP"); dir != "" {
								_ = os.WriteFile(filepath.Join(dir, "slimebot-probe-screen.jpg"), data, 0644)
							}
							return snapshot.ID, img
						}
					}
				}
			}
			time.Sleep(150 * time.Millisecond)
		}
		t.Fatalf("native Wayland window did not render in a screenshot: %v", err)
		return "", nil
	}

	waitResult := func(done <-chan error, stdout, stderr *bytes.Buffer, expected string) {
		select {
		case err := <-done:
			if err != nil || strings.TrimSpace(stdout.String()) != expected {
				t.Fatalf("Wayland probe result: err=%v stdout=%q stderr=%q, want=%q", err, stdout.String(), stderr.String(), expected)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("Wayland probe did not receive input; stdout=%q stderr=%q", stdout.String(), stderr.String())
		}
	}
	focusWindow := func(mode string) {
		match := "title:SlimeBot Wayland " + mode
		waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
		defer cancelWait()
		out, err := exec.CommandContext(waitCtx, "wlrctl", "toplevel", "waitfor", match).CombinedOutput()
		if err != nil {
			t.Fatalf("wait for native Wayland %s window: %v: %s", mode, err, out)
		}
		out, err = exec.CommandContext(ctx, "wlrctl", "toplevel", "focus", match).CombinedOutput()
		if err != nil {
			t.Fatalf("focus native Wayland %s window: %v: %s", mode, err, out)
		}
		time.Sleep(100 * time.Millisecond)
	}

	entryDone, entryOut, entryErr := startProbe("entry")
	focusWindow("entry")
	id, img := screenshot()
	t.Logf("entry screenshot: %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	center := img.Bounds().Min.Add(img.Bounds().Size().Div(2))
	if _, err := tool.Execute(ctx, "click", map[string]any{"snapshot_id": id, "x": center.X, "y": center.Y}); err != nil {
		t.Fatalf("focus native entry with /dev/uinput click: %v", err)
	}
	active, err := exec.CommandContext(ctx, "wlrctl", "toplevel", "find", "title:SlimeBot Wayland entry", "state:active").CombinedOutput()
	t.Logf("entry active after /dev/uinput click: %v %s", err, active)
	id, _ = screenshot()
	if _, err := tool.Execute(ctx, "type", map[string]any{"snapshot_id": id, "text": "Wayland 43"}); err != nil {
		t.Fatalf("real /dev/uinput text input: %v", err)
	}
	id, _ = screenshot()
	if _, err := tool.Execute(ctx, "key", map[string]any{"snapshot_id": id, "key": "Backspace"}); err != nil {
		t.Fatalf("real /dev/uinput Backspace: %v", err)
	}
	id, _ = screenshot()
	if _, err := tool.Execute(ctx, "key", map[string]any{"snapshot_id": id, "key": "Enter"}); err != nil {
		t.Fatalf("real /dev/uinput Enter: %v", err)
	}
	waitResult(entryDone, entryOut, entryErr, "Wayland 4")

	buttonDone, buttonOut, buttonErr := startProbe("button")
	focusWindow("button")
	id, img = screenshot()
	t.Logf("button screenshot: %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	center = img.Bounds().Min.Add(img.Bounds().Size().Div(2))
	if _, err := tool.Execute(ctx, "click", map[string]any{"snapshot_id": id, "x": center.X, "y": center.Y}); err != nil {
		t.Fatalf("real /dev/uinput coordinate click: %v", err)
	}
	waitResult(buttonDone, buttonOut, buttonErr, "clicked")
	t.Logf("Wayland compositor screenshot and /dev/uinput keyboard/mouse input reached native GTK controls (%dx%d)", img.Bounds().Dx(), img.Bounds().Dy())
}
