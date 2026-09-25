//go:build linux

package tools

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Exercises the Go-to-Python Wayland path with command stand-ins; no compositor is needed.
func TestComputerLinuxWaylandCommands(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(python, filepath.Join(bin, "python3")); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "calls.log")
	pngPath := filepath.Join(dir, "screen.png")
	imageFile, err := os.Create(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 2, color.RGBA{R: 255, A: 255})
	if err := png.Encode(imageFile, img); err != nil {
		t.Fatal(err)
	}
	if err := imageFile.Close(); err != nil {
		t.Fatal(err)
	}
	const logArgs = "printf '%s' \"${0##*/}\" >> \"$MOCK_LOG\"\nprintf '|%s' \"$@\" >> \"$MOCK_LOG\"\nprintf '\\n' >> \"$MOCK_LOG\"\n"
	if err := os.WriteFile(filepath.Join(bin, "grim"), []byte("#!/bin/sh\n"+logArgs+"/bin/cp \"$MOCK_PNG\" \"$1\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "ydotool"), []byte("#!/bin/sh\nif [ \"$1\" = help ]; then echo 'debug'; exit 1; fi\n"+logArgs+"if [ -f \"$MOCK_FAIL\" ]; then echo 'ydotoold unavailable' >&2; exit 1; fi\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")
	t.Setenv("DISPLAY", ":99") // Wayland must win when Xwayland also sets DISPLAY.
	t.Setenv("MOCK_LOG", logPath)
	t.Setenv("MOCK_PNG", pngPath)
	t.Setenv("MOCK_FAIL", filepath.Join(dir, "fail"))
	ctx := context.Background()
	tool := &computerTool{snapshots: make(map[string]*computerSnapshot)}
	result, err := tool.Execute(ctx, "observe", map[string]any{"mode": "screenshot"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result.ImageURL, "data:image/jpeg;base64,") {
		t.Fatal("Wayland screenshot did not reach the Go image conversion")
	}
	var snapshot struct {
		ID     string `json:"snapshot_id"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal([]byte(result.Output), &snapshot); err != nil || snapshot.Source != "screenshot" || snapshot.ID == "" {
		t.Fatalf("invalid screenshot result: %v: %s", err, result.Output)
	}
	if _, err := tool.Execute(ctx, "click", map[string]any{"snapshot_id": snapshot.ID, "x": 1, "y": 2}); err != nil {
		t.Fatal(err)
	}
	for _, req := range []computerRequest{
		{Op: "type", Text: "literal \\n -x"},
		{Op: "key", Key: "Backspace"},
		{Op: "scroll", Direction: "down", Amount: 2},
	} {
		if _, err := runComputerHelper(ctx, req); err != nil {
			t.Fatalf("%s: %v", req.Op, err)
		}
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(got) == 0 || !strings.HasPrefix(got[0], "grim|") || !strings.Contains(got[0], "slimebot-computer-") || !strings.HasSuffix(got[0], ".png") {
		t.Fatalf("grim was not called with a screenshot path: %q", got)
	}
	want := []string{
		"ydotool|mousemove|--absolute|2|3",
		"ydotool|click|0xC0",
		"ydotool|type|--|literal \\n -x",
		"ydotool|key|14:1|14:0",
		"ydotool|key|109:1|109:0",
		"ydotool|key|109:1|109:0",
	}
	if !reflect.DeepEqual(got[1:], want) {
		t.Fatalf("Wayland command sequence:\n got: %q\nwant: %q", got[1:], want)
	}
	if err := os.WriteFile(os.Getenv("MOCK_FAIL"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	_, err = runComputerHelper(ctx, computerRequest{Op: "key", Key: "Enter"})
	if err == nil || !strings.Contains(err.Error(), "ydotoold unavailable") {
		t.Fatalf("ydotoold failure was not reported: %v", err)
	}
}

func TestComputerLinuxWaylandLegacyYdotoolCommands(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(python, filepath.Join(bin, "python3")); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "calls.log")
	const mock = "#!/bin/sh\nif [ \"$1\" = help ]; then echo 'recorder'; exit 1; fi\nprintf '%s' \"${0##*/}\" >> \"$MOCK_LOG\"\nprintf '|%s' \"$@\" >> \"$MOCK_LOG\"\nprintf '\\n' >> \"$MOCK_LOG\"\n"
	if err := os.WriteFile(filepath.Join(bin, "ydotool"), []byte(mock), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")
	t.Setenv("MOCK_LOG", logPath)
	for _, req := range []computerRequest{
		{Op: "click", X: 12, Y: 34},
		{Op: "type", Text: "Wayland 43"},
		{Op: "key", Key: "Backspace"},
		{Op: "key", Key: "Enter"},
		{Op: "scroll", Direction: "down", Amount: 1},
	} {
		if _, err := runComputerHelper(context.Background(), req); err != nil {
			t.Fatalf("%s: %v", req.Op, err)
		}
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := []string{
		"ydotool|mousemove|12|34", "ydotool|click|1", "ydotool|type|--|Wayland 43",
		"ydotool|key|Backspace", "ydotool|key|Enter", "ydotool|key|PageDown",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy ydotool command sequence:\n got: %q\nwant: %q", got, want)
	}
}

func TestComputerLinuxWaylandSpectacleScreenshot(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(python, filepath.Join(bin, "python3")); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 2, color.RGBA{R: 255, A: 255})
	imagePath := filepath.Join(dir, "source.png")
	imageFile, err := os.Create(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(imageFile, img); err != nil {
		t.Fatal(err)
	}
	if err := imageFile.Close(); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "calls.log")
	const mock = "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$MOCK_LOG\"\n/bin/cp \"$MOCK_PNG\" \"$5\"\n"
	if err := os.WriteFile(filepath.Join(bin, "spectacle"), []byte(mock), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")
	t.Setenv("XDG_CURRENT_DESKTOP", "KDE")
	t.Setenv("MOCK_LOG", logPath)
	t.Setenv("MOCK_PNG", imagePath)
	tool := &computerTool{snapshots: make(map[string]*computerSnapshot)}
	result, err := tool.Execute(context.Background(), "observe", map[string]any{"mode": "screenshot"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result.ImageURL, "data:image/jpeg;base64,") {
		t.Fatal("Spectacle screenshot did not reach the Go image conversion")
	}
	calls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(calls), "-b -n -f -o ") {
		t.Fatalf("unexpected Spectacle command: %q", calls)
	}
}
