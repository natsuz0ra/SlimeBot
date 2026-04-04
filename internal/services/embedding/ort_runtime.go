package embedding

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	runtimecfg "slimebot/internal/runtime"

	ort "github.com/yalue/onnxruntime_go"
)

const defaultORTDownloadBaseURL = "https://github.com/microsoft/onnxruntime/releases/download"

type ORTRuntimeConfig struct {
	Version         string
	CacheDir        string
	LibPath         string
	DownloadBaseURL string
}

func EnsureORTSharedLibrary(ctx context.Context, cfg ORTRuntimeConfig) (string, error) {
	if p := strings.TrimSpace(cfg.LibPath); p != "" {
		abs := absIfRel(p)
		if !isFile(abs) {
			return "", fmt.Errorf("onnxruntime shared library not found: %s", abs)
		}
		return abs, nil
	}
	version := strings.TrimSpace(cfg.Version)
	if version == "" {
		version = "1.24.1"
	}
	cacheDir := absIfRel(strings.TrimSpace(cfg.CacheDir))
	if cacheDir == "" {
		cacheDir = absIfRel(filepath.Join(runtimecfg.SlimeBotHomeDir(), "onnx", "runtime"))
	}
	if err := os.MkdirAll(cacheDir, os.ModePerm); err != nil {
		return "", err
	}
	baseURL := strings.TrimSpace(cfg.DownloadBaseURL)
	if baseURL == "" {
		baseURL = defaultORTDownloadBaseURL
	}

	assetName, err := resolveORTAssetName(runtime.GOOS, runtime.GOARCH, version)
	if err != nil {
		return "", err
	}
	slog.Info("resource_prepare_start",
		"resource", "onnxruntime",
		"asset", assetName,
		"cache_dir", cacheDir,
	)
	extractedRoot := filepath.Join(cacheDir, archiveStem(assetName))
	libPath, _ := findExtractedORTLibrary(extractedRoot, version)
	if libPath != "" {
		slog.Info("resource_prepare_done",
			"resource", "onnxruntime",
			"asset", assetName,
			"cached", true,
			"lib_path", libPath,
		)
		return libPath, nil
	}

	archivePath := filepath.Join(cacheDir, assetName)
	if !isFile(archivePath) {
		downloadURL := fmt.Sprintf("%s/v%s/%s", strings.TrimRight(baseURL, "/"), version, assetName)
		if err := downloadFile(ctx, downloadURL, archivePath); err != nil {
			slog.Warn("resource_prepare_failed",
				"resource", "onnxruntime",
				"asset", assetName,
				"stage", "download",
				"err", err,
			)
			return "", err
		}
	}
	if err := extractORTArchive(archivePath, cacheDir); err != nil {
		slog.Warn("resource_prepare_failed",
			"resource", "onnxruntime",
			"asset", assetName,
			"stage", "extract",
			"err", err,
		)
		return "", err
	}
	libPath, err = findExtractedORTLibrary(extractedRoot, version)
	if err != nil {
		slog.Warn("resource_prepare_failed",
			"resource", "onnxruntime",
			"asset", assetName,
			"stage", "find_library",
			"err", err,
		)
		return "", err
	}
	slog.Info("resource_prepare_done",
		"resource", "onnxruntime",
		"asset", assetName,
		"cached", false,
		"lib_path", libPath,
	)
	return libPath, nil
}

func resolveORTAssetName(goos, goarch, version string) (string, error) {
	switch goos {
	case "windows":
		switch goarch {
		case "amd64":
			return fmt.Sprintf("onnxruntime-win-x64-%s.zip", version), nil
		case "arm64":
			return fmt.Sprintf("onnxruntime-win-arm64-%s.zip", version), nil
		}
	case "linux":
		switch goarch {
		case "amd64":
			return fmt.Sprintf("onnxruntime-linux-x64-%s.tgz", version), nil
		case "arm64":
			return fmt.Sprintf("onnxruntime-linux-aarch64-%s.tgz", version), nil
		}
	case "darwin":
		if goarch == "arm64" {
			return fmt.Sprintf("onnxruntime-osx-arm64-%s.tgz", version), nil
		}
	}
	return "", fmt.Errorf("unsupported os/arch for onnxruntime: %s/%s", goos, goarch)
}

func downloadFile(ctx context.Context, url, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s failed: status=%d", url, resp.StatusCode)
	}
	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func extractORTArchive(archivePath, dstDir string) error {
	if strings.HasSuffix(strings.ToLower(archivePath), ".zip") {
		return extractZip(archivePath, dstDir)
	}
	if strings.HasSuffix(strings.ToLower(archivePath), ".tgz") || strings.HasSuffix(strings.ToLower(archivePath), ".tar.gz") {
		return extractTGZ(archivePath, dstDir)
	}
	return fmt.Errorf("unsupported archive format: %s", archivePath)
}

func archiveStem(assetName string) string {
	name := strings.TrimSuffix(assetName, ".zip")
	name = strings.TrimSuffix(name, ".tgz")
	name = strings.TrimSuffix(name, ".tar.gz")
	return name
}

func extractZip(archivePath, dstDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		target := filepath.Join(dstDir, filepath.Clean(f.Name))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, os.ModePerm); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), os.ModePerm); err != nil {
			return err
		}
		in, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			in.Close()
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			in.Close()
			return err
		}
		out.Close()
		in.Close()
	}
	return nil
}

func extractTGZ(archivePath, dstDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, filepath.Clean(h.Name))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.ModePerm); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), os.ModePerm); err != nil {
				return err
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}

func findExtractedORTLibrary(rootDir, version string) (string, error) {
	wantNames := []string{}
	switch runtime.GOOS {
	case "windows":
		wantNames = append(wantNames, "onnxruntime.dll")
	case "darwin":
		wantNames = append(wantNames, "libonnxruntime.dylib")
	case "linux":
		wantNames = append(wantNames, "libonnxruntime.so."+version, "libonnxruntime.so")
	default:
		return "", fmt.Errorf("unsupported os: %s", runtime.GOOS)
	}
	var found string
	_ = filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := filepath.Base(path)
		for _, want := range wantNames {
			if name == want {
				found = path
				return io.EOF
			}
		}
		return nil
	})
	if found == "" {
		return "", fmt.Errorf("onnxruntime shared library not found under %s", rootDir)
	}
	return found, nil
}

var ortEnvMu sync.Mutex
var ortEnvRefCount int

func acquireORTEnvironment(sharedLibPath string) error {
	ortEnvMu.Lock()
	defer ortEnvMu.Unlock()
	if ortEnvRefCount == 0 {
		ort.SetSharedLibraryPath(sharedLibPath)
		if err := ort.InitializeEnvironment(); err != nil {
			return fmt.Errorf("initialize onnxruntime environment failed: %w", err)
		}
	}
	ortEnvRefCount++
	return nil
}

func releaseORTEnvironment() error {
	ortEnvMu.Lock()
	defer ortEnvMu.Unlock()
	if ortEnvRefCount == 0 {
		return nil
	}
	ortEnvRefCount--
	if ortEnvRefCount == 0 {
		if err := ort.DestroyEnvironment(); err != nil {
			return err
		}
	}
	return nil
}
