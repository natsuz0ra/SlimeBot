package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestComputerImageResizesAndKeepsScreenshotCoordinates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "screen.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 4096, 2048))
	img.Set(100, 100, color.RGBA{R: 255, A: 255})
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	imageURL, originalW, originalH, imageW, imageH, err := computerImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if originalW != 4096 || originalH != 2048 || imageW != 2048 || imageH != 1024 {
		t.Fatalf("unexpected dimensions: %d,%d -> %d,%d", originalW, originalH, imageW, imageH)
	}
	if !strings.HasPrefix(imageURL, "data:image/jpeg;base64,") {
		t.Fatalf("missing image data URL: %q", imageURL[:min(len(imageURL), 30)])
	}
	if _, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(imageURL, "data:image/jpeg;base64,")); err != nil {
		t.Fatal(err)
	}
}

func TestComputerActionRejectsExpiredOrForeignSnapshot(t *testing.T) {
	tool := &computerTool{snapshots: map[string]*computerSnapshot{
		"expired": {sessionID: "local", expires: time.Now().Add(-time.Second)},
		"foreign": {sessionID: "other", expires: time.Now().Add(time.Minute)},
	}}
	for _, id := range []string{"expired", "foreign"} {
		_, err := tool.Execute(context.Background(), "key", map[string]any{"snapshot_id": id, "key": "Enter"})
		if err == nil || !strings.Contains(err.Error(), "observe again") {
			t.Fatalf("%s: expected stale snapshot error, got %v", id, err)
		}
	}
}

func TestComputerCommandsAreAvailableToModel(t *testing.T) {
	if !IsApprovalSensitiveTool("computer") {
		t.Fatal("computer actions must follow the approval policy")
	}
	seen := make(map[string]bool)
	for _, def := range BuildRegistryToolDefs() {
		seen[def.Name] = true
	}
	for _, name := range []string{"computer__observe", "computer__click", "computer__type", "computer__key", "computer__scroll"} {
		if !seen[name] {
			t.Fatalf("missing model tool definition %s", name)
		}
	}
}

func TestComputerObserveLive(t *testing.T) {
	if os.Getenv("SLIMEBOT_COMPUTER_LIVE_TEST") != "1" || runtime.GOOS != "darwin" {
		t.Skip("set SLIMEBOT_COMPUTER_LIVE_TEST=1 on macOS to capture the local desktop")
	}
	tool := &computerTool{snapshots: make(map[string]*computerSnapshot)}
	result, err := tool.Execute(context.Background(), "observe", map[string]any{"mode": "auto"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "snapshot_id") {
		t.Fatalf("missing snapshot ID: %s", result.Output)
	}
	if strings.Contains(result.Output, `"source":"accessibility"`) {
		t.Log("observation source: accessibility")
	} else if strings.Contains(result.Output, `"source":"screenshot"`) {
		t.Log("observation source: screenshot")
	}
	if strings.Contains(result.Output, `"source":"screenshot"`) && !strings.HasPrefix(result.ImageURL, "data:image/jpeg;base64,") {
		t.Fatal("screenshot fallback did not return an image to the model")
	}
}

func TestComputerActionLive(t *testing.T) {
	if os.Getenv("SLIMEBOT_COMPUTER_ACTION_TEST") != "1" || runtime.GOOS != "darwin" {
		t.Skip("set SLIMEBOT_COMPUTER_ACTION_TEST=1 on macOS to test a temporary dialog")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const marker = "SlimeBot Computer Action Test"
	cmd := exec.CommandContext(ctx, "/usr/bin/osascript", "-e",
		`display dialog "`+marker+`" default answer "" buttons {"Cancel", "OK"} default button "OK" with title "`+marker+`"`)
	var dialogOutput bytes.Buffer
	cmd.Stdout, cmd.Stderr = &dialogOutput, &dialogOutput
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	completed := false
	defer func() {
		if !completed {
			cancel()
			<-wait
		}
	}()

	tool := &computerTool{snapshots: make(map[string]*computerSnapshot)}
	var observation struct {
		SnapshotID string         `json:"snapshot_id"`
		Source     string         `json:"source"`
		Window     string         `json:"window"`
		Nodes      []computerNode `json:"nodes"`
	}
	observeDialog := func() {
		for ctx.Err() == nil {
			result, err := tool.Execute(ctx, "observe", map[string]any{"mode": "accessibility"})
			if err == nil && json.Unmarshal([]byte(result.Output), &observation) == nil && observation.Source == "accessibility" && observation.Window == marker {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("test dialog did not expose an accessibility tree")
	}
	findRef := func(role, name string) string {
		for _, node := range observation.Nodes {
			if node.Role == role && (name == "" || node.Name == name) {
				return node.Ref
			}
		}
		t.Fatalf("test dialog has no %s %q", role, name)
		return ""
	}

	observeDialog()
	field := findRef("AXTextField", "")
	if _, err := tool.Execute(ctx, "type", map[string]any{"snapshot_id": observation.SnapshotID, "ref": field, "text": "SlimeBot 42"}); err != nil {
		t.Fatal(err)
	}
	observeDialog()
	button := findRef("AXButton", "OK")
	if _, err := tool.Execute(ctx, "click", map[string]any{"snapshot_id": observation.SnapshotID, "ref": button}); err != nil {
		t.Fatal(err)
	}
	if err := <-wait; err != nil {
		t.Fatalf("test dialog failed: %v: %s", err, dialogOutput.String())
	}
	completed = true
	if !strings.Contains(dialogOutput.String(), "text returned:SlimeBot 42") {
		t.Fatalf("dialog did not receive entered text: %s", dialogOutput.String())
	}
	t.Log("accessibility text entry and AXPress click succeeded")
}

func TestComputerScreenshotActionLive(t *testing.T) {
	if os.Getenv("SLIMEBOT_COMPUTER_SCREENSHOT_ACTION_TEST") != "1" || runtime.GOOS != "darwin" {
		t.Skip("set SLIMEBOT_COMPUTER_SCREENSHOT_ACTION_TEST=1 on macOS to test screenshot coordinates")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const marker = "SlimeBot Screenshot Action Test"
	cmd := exec.CommandContext(ctx, "/usr/bin/osascript", "-e",
		`display dialog "`+marker+`" default answer "" buttons {"Cancel", "OK"} default button "OK" with title "`+marker+`"`)
	var dialogOutput bytes.Buffer
	cmd.Stdout, cmd.Stderr = &dialogOutput, &dialogOutput
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	completed := false
	defer func() {
		if !completed {
			cancel()
			<-wait
		}
	}()
	var nodes []computerNode
	for ctx.Err() == nil {
		observed, err := runComputerHelper(ctx, computerRequest{Op: "observe", MaxNodes: 120})
		if err == nil && observed.Window == marker {
			nodes = observed.Nodes
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(nodes) == 0 {
		t.Fatal("test dialog did not expose accessibility controls for locating screenshot coordinates")
	}
	findBounds := func(role, name string) computerResponse {
		for _, node := range nodes {
			if node.Role == role && (name == "" || node.Name == name) {
				bounds, err := runComputerHelper(ctx, computerRequest{Op: "bounds", Target: &node})
				if err != nil {
					t.Fatal(err)
				}
				return bounds
			}
		}
		t.Fatalf("test dialog has no %s %q", role, name)
		return computerResponse{}
	}
	field := findBounds("AXTextField", "")
	button := findBounds("AXButton", "OK")
	tool := &computerTool{snapshots: make(map[string]*computerSnapshot)}
	observeScreenshot := func() string {
		result, err := tool.Execute(ctx, "observe", map[string]any{"mode": "screenshot"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(result.ImageURL, "data:image/jpeg;base64,") {
			t.Fatal("screenshot observation did not return an image")
		}
		data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(result.ImageURL, "data:image/jpeg;base64,"))
		if err != nil {
			t.Fatal(err)
		}
		imageConfig, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil || imageConfig.Width == 0 || imageConfig.Height == 0 {
			t.Fatalf("screenshot is not a valid JPEG: %v", err)
		}
		var observation struct {
			SnapshotID string `json:"snapshot_id"`
			Source     string `json:"source"`
		}
		if err := json.Unmarshal([]byte(result.Output), &observation); err != nil || observation.Source != "screenshot" {
			t.Fatalf("invalid screenshot observation: %v: %s", err, result.Output)
		}
		coord := tool.snapshots[observation.SnapshotID].coord
		if imageConfig.Width != coord.imageWidth || imageConfig.Height != coord.imageHeight {
			t.Fatalf("screenshot dimensions %dx%d do not match coordinate map %dx%d", imageConfig.Width, imageConfig.Height, coord.imageWidth, coord.imageHeight)
		}
		return observation.SnapshotID
	}
	clickBounds := func(snapshotID string, bounds computerResponse) {
		coord := tool.snapshots[snapshotID].coord
		x := int(math.Floor((bounds.ScreenX + bounds.ScreenWidth/2 - coord.screenX) * float64(coord.imageWidth) / coord.screenWidth))
		y := int(math.Floor((bounds.ScreenY + bounds.ScreenHeight/2 - coord.screenY) * float64(coord.imageHeight) / coord.screenHeight))
		if x < 0 || y < 0 || x >= coord.imageWidth || y >= coord.imageHeight {
			t.Fatalf("test control is outside screenshot: %d,%d", x, y)
		}
		if _, err := tool.Execute(ctx, "click", map[string]any{"snapshot_id": snapshotID, "x": x, "y": y}); err != nil {
			t.Fatal(err)
		}
	}

	clickBounds(observeScreenshot(), field)
	if _, err := tool.Execute(ctx, "type", map[string]any{"snapshot_id": observeScreenshot(), "text": "Screenshot 42"}); err != nil {
		t.Fatal(err)
	}
	clickBounds(observeScreenshot(), button)
	if err := <-wait; err != nil {
		t.Fatalf("test dialog failed: %v: %s", err, dialogOutput.String())
	}
	completed = true
	if !strings.Contains(dialogOutput.String(), "text returned:Screenshot 42") {
		t.Fatalf("dialog did not receive screenshot-based input: %s", dialogOutput.String())
	}
	t.Log("screenshot coordinate clicks and focused text input succeeded")
}
