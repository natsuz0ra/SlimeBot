//go:build linux

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestComputerLinuxX11Live(t *testing.T) {
	if os.Getenv("SLIMEBOT_COMPUTER_LINUX_LIVE_TEST") != "1" {
		t.Skip("set SLIMEBOT_COMPUTER_LINUX_LIVE_TEST=1 in an X11 desktop session")
	}
	if os.Getenv("DISPLAY") == "" {
		t.Fatal("DISPLAY is required for the X11 live test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	startDialog := func(title, program string) (<-chan error, *bytes.Buffer) {
		args := []string{"--entry", "--title=" + title, "--text=Computer Use smoke test"}
		if program == "yad" {
			args = append(args, "--button=Commit:0", "--button=Cancel:1")
		} else {
			args = append(args, "--ok-label=Commit")
		}
		cmd := exec.CommandContext(ctx, program, args...)
		output := &bytes.Buffer{}
		cmd.Stdout = output
		cmd.Stderr = &bytes.Buffer{}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		wait := make(chan error, 1)
		go func() { wait <- cmd.Wait() }()
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) && ctx.Err() == nil {
			found, err := exec.CommandContext(ctx, "xdotool", "search", "--onlyvisible", "--name", title).Output()
			if err == nil && len(strings.Fields(string(found))) > 0 {
				id := strings.Fields(string(found))[0]
				if err := exec.CommandContext(ctx, "xdotool", "windowactivate", "--sync", id).Run(); err == nil {
					return wait, output
				}
			}
			time.Sleep(150 * time.Millisecond)
		}
		t.Fatalf("test dialog %q did not become active", title)
		return nil, nil
	}

	observeAX := func(tool *computerTool, title string) (string, []computerNode) {
		deadline := time.Now().Add(20 * time.Second)
		var lastError error
		var lastWindow string
		var lastNodes []computerNode
		for time.Now().Before(deadline) && ctx.Err() == nil {
			result, err := tool.Execute(ctx, "observe", map[string]any{"mode": "accessibility"})
			if err == nil {
				var observed struct {
					SnapshotID string         `json:"snapshot_id"`
					Window     string         `json:"window"`
					Nodes      []computerNode `json:"nodes"`
				}
				if err = json.Unmarshal([]byte(result.Output), &observed); err == nil && observed.Window == title {
					return observed.SnapshotID, observed.Nodes
				}
				lastWindow, lastNodes = observed.Window, observed.Nodes
			}
			lastError = err
			time.Sleep(150 * time.Millisecond)
		}
		focused, _ := exec.CommandContext(ctx, "xdotool", "getwindowfocus", "getwindowname").CombinedOutput()
		accessibilityTree, _ := exec.CommandContext(ctx, "python3", "-c", "import pyatspi; d=pyatspi.Registry.getDesktop(0); print([(a.name, [(w.name, w.getRoleName()) for w in a]) for a in d])").CombinedOutput()
		t.Fatalf("AT-SPI did not expose test dialog %q: last error=%v, window=%q, nodes=%+v, X11 focus=%q, AT-SPI desktop=%q", title, lastError, lastWindow, lastNodes, strings.TrimSpace(string(focused)), strings.TrimSpace(string(accessibilityTree)))
		return "", nil
	}
	findNode := func(nodes []computerNode, rolePart, name string) computerNode {
		for _, node := range nodes {
			if strings.Contains(strings.ToLower(node.Role), rolePart) && (name == "" || node.Name == name) {
				return node
			}
		}
		t.Fatalf("AT-SPI tree has no %s %q: %+v", rolePart, name, nodes)
		return computerNode{}
	}

	const axTitle = "SlimeBot Linux X11 Accessibility Smoke"
	axWait, axOutput := startDialog(axTitle, "zenity")
	tool := &computerTool{snapshots: make(map[string]*computerSnapshot)}
	id, nodes := observeAX(tool, axTitle)
	field := findNode(nodes, "text", "")
	if _, err := tool.Execute(ctx, "type", map[string]any{"snapshot_id": id, "ref": field.Ref, "text": "Linux AX 42"}); err != nil {
		t.Fatalf("AT-SPI text input: %v", err)
	}
	id, nodes = observeAX(tool, axTitle)
	button := findNode(nodes, "button", "Commit")
	if _, err := tool.Execute(ctx, "click", map[string]any{"snapshot_id": id, "ref": button.Ref}); err != nil {
		t.Fatalf("AT-SPI button click: %v", err)
	}
	if err := <-axWait; err != nil || strings.TrimSpace(axOutput.String()) != "Linux AX 42" {
		t.Fatalf("AT-SPI dialog result: err=%v output=%q", err, axOutput.String())
	}

	const pixelTitle = "SlimeBot Linux X11 Screenshot Smoke"
	pixelWait, pixelOutput := startDialog(pixelTitle, "yad")
	_, nodes = observeAX(tool, pixelTitle)
	field = findNode(nodes, "text", "")
	button = findNode(nodes, "button", "Commit")
	fieldBounds, err := runComputerHelper(ctx, computerRequest{Op: "bounds", Target: &field})
	if err != nil {
		t.Fatalf("text field bounds: %v", err)
	}
	buttonBounds, err := runComputerHelper(ctx, computerRequest{Op: "bounds", Target: &button})
	if err != nil {
		t.Fatalf("button bounds: %v", err)
	}
	t.Logf("pixel controls: field=(%.0f,%.0f %.0fx%.0f) button=(%.0f,%.0f %.0fx%.0f)",
		fieldBounds.ScreenX, fieldBounds.ScreenY, fieldBounds.ScreenWidth, fieldBounds.ScreenHeight,
		buttonBounds.ScreenX, buttonBounds.ScreenY, buttonBounds.ScreenWidth, buttonBounds.ScreenHeight)
	if fieldBounds.ScreenX == 0 && fieldBounds.ScreenY == 0 && buttonBounds.ScreenX == 0 && buttonBounds.ScreenY == 0 {
		t.Fatal("AT-SPI returned only origin coordinates for the test controls")
	}
	observeScreenshot := func() string {
		result, err := tool.Execute(ctx, "observe", map[string]any{"mode": "screenshot"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(result.ImageURL, "data:image/jpeg;base64,") {
			t.Fatal("screenshot observation did not return a JPEG")
		}
		var observed struct {
			SnapshotID string `json:"snapshot_id"`
			Source     string `json:"source"`
		}
		if err := json.Unmarshal([]byte(result.Output), &observed); err != nil || observed.Source != "screenshot" {
			t.Fatalf("invalid screenshot observation: %v: %s", err, result.Output)
		}
		return observed.SnapshotID
	}
	clickBounds := func(snapshotID string, bounds computerResponse) {
		coord := tool.snapshots[snapshotID].coord
		if coord.imageWidth <= 0 || coord.imageHeight <= 0 || bounds.ScreenWidth <= 0 || bounds.ScreenHeight <= 0 {
			t.Fatalf("invalid screenshot or control bounds: %+v, %+v", coord, bounds)
		}
		x := int(math.Floor((bounds.ScreenX + bounds.ScreenWidth/2 - coord.screenX) * float64(coord.imageWidth) / coord.screenWidth))
		y := int(math.Floor((bounds.ScreenY + bounds.ScreenHeight/2 - coord.screenY) * float64(coord.imageHeight) / coord.screenHeight))
		t.Logf("click screenshot coordinate (%d,%d) from %dx%d image", x, y, coord.imageWidth, coord.imageHeight)
		if _, err := tool.Execute(ctx, "click", map[string]any{"snapshot_id": snapshotID, "x": x, "y": y}); err != nil {
			t.Fatal(err)
		}
	}
	clickBounds(observeScreenshot(), fieldBounds)
	if _, err := tool.Execute(ctx, "type", map[string]any{"snapshot_id": observeScreenshot(), "text": "Linux pixel 423"}); err != nil {
		t.Fatalf("X11 focused text input: %v", err)
	}
	if _, err := tool.Execute(ctx, "key", map[string]any{"snapshot_id": observeScreenshot(), "key": "Backspace"}); err != nil {
		t.Fatalf("X11 keyboard action: %v", err)
	}
	var entered computerResponse
	for attempt := 0; attempt < 10; attempt++ {
		entered, err = runComputerHelper(ctx, computerRequest{Op: "read", Target: &field})
		if err != nil || entered.Message == "Linux pixel 42" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil || entered.Message != "Linux pixel 42" {
		t.Fatalf("screenshot input did not reach text field: value=%q err=%v", entered.Message, err)
	}
	clickBounds(observeScreenshot(), buttonBounds)
	select {
	case err := <-pixelWait:
		if err != nil || strings.TrimSpace(pixelOutput.String()) != "Linux pixel 42" {
			t.Fatalf("screenshot dialog result: err=%v output=%q", err, pixelOutput.String())
		}
	case <-time.After(5 * time.Second):
		focused, _ := exec.CommandContext(ctx, "xdotool", "getwindowfocus", "getwindowname").CombinedOutput()
		pointer, _ := exec.CommandContext(ctx, "xdotool", "getmouselocation").CombinedOutput()
		t.Fatalf("screenshot button click did not close dialog: X11 focus=%q pointer=%q", strings.TrimSpace(string(focused)), strings.TrimSpace(string(pointer)))
	}
	t.Log("X11 AT-SPI observation/actions and screenshot coordinate actions succeeded")
}
