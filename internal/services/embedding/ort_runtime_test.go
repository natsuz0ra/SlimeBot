package embedding

import "testing"

func TestResolveORTAssetName(t *testing.T) {
	tests := []struct {
		goos    string
		goarch  string
		version string
		want    string
	}{
		{"windows", "amd64", "1.24.1", "onnxruntime-win-x64-1.24.1.zip"},
		{"linux", "amd64", "1.24.1", "onnxruntime-linux-x64-1.24.1.tgz"},
		{"linux", "arm64", "1.24.1", "onnxruntime-linux-aarch64-1.24.1.tgz"},
		{"darwin", "arm64", "1.24.1", "onnxruntime-osx-arm64-1.24.1.tgz"},
	}
	for _, tt := range tests {
		got, err := resolveORTAssetName(tt.goos, tt.goarch, tt.version)
		if err != nil {
			t.Fatalf("unexpected error for %s/%s: %v", tt.goos, tt.goarch, err)
		}
		if got != tt.want {
			t.Fatalf("asset mismatch for %s/%s: got=%s want=%s", tt.goos, tt.goarch, got, tt.want)
		}
	}
}
