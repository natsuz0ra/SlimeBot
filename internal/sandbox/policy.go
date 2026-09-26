package sandbox

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Mode string

const (
	ModeReadOnly         Mode = "read-only"
	ModeWorkspaceWrite   Mode = "workspace-write"
	ModeDangerFullAccess Mode = "danger-full-access"
)

type NetworkPolicy struct {
	Enabled        bool
	AllowedDomains []string
}

type Config struct {
	Mode          Mode
	CWD           string
	WritableRoots []string
	DenyPaths     []string
	Network       NetworkPolicy
}

type Policy struct {
	mode          Mode
	cwd           string
	writableRoots []string
	denyPaths     []string
	network       NetworkPolicy
}

type contextKey struct{}
type escalationGrantKey struct{}
type workingDirectoryKey struct{}

func WithWorkingDirectory(ctx context.Context, directory string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if directory == "" {
		return ctx
	}
	return context.WithValue(ctx, workingDirectoryKey{}, directory)
}

func WorkingDirectoryFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	directory, _ := ctx.Value(workingDirectoryKey{}).(string)
	return directory
}

func NewPolicy(cfg Config) (*Policy, error) {
	mode := cfg.Mode
	if mode == "" {
		mode = ModeWorkspaceWrite
	}
	if mode != ModeReadOnly && mode != ModeWorkspaceWrite && mode != ModeDangerFullAccess {
		return nil, fmt.Errorf("invalid sandbox mode: %s", mode)
	}
	cwd := strings.TrimSpace(cfg.CWD)
	if cwd == "" {
		got, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("resolve sandbox cwd: %w", err)
		}
		cwd = got
	}
	cwd, err := normalizePath(cwd)
	if err != nil {
		return nil, fmt.Errorf("resolve sandbox cwd: %w", err)
	}
	p := &Policy{mode: mode, cwd: cwd, network: cfg.Network}
	if mode == ModeWorkspaceWrite {
		p.writableRoots = append(p.writableRoots, cwd)
	}
	for _, root := range cfg.WritableRoots {
		normalized, err := normalizePath(root)
		if err != nil {
			return nil, fmt.Errorf("resolve writable root %q: %w", root, err)
		}
		p.writableRoots = appendUniquePath(p.writableRoots, normalized)
	}
	for _, path := range cfg.DenyPaths {
		normalized, err := normalizePath(path)
		if err != nil {
			return nil, fmt.Errorf("resolve deny path %q: %w", path, err)
		}
		p.denyPaths = appendUniquePath(p.denyPaths, normalized)
	}
	return p, nil
}

func DefaultPolicy(cwd string) (*Policy, error) {
	return NewPolicy(Config{
		Mode:    ModeWorkspaceWrite,
		CWD:     cwd,
		Network: NetworkPolicy{Enabled: true},
	})
}

func WithPolicy(ctx context.Context, policy *Policy) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if policy == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, policy)
}

func FromContext(ctx context.Context) (*Policy, bool) {
	if ctx == nil {
		return nil, false
	}
	p, ok := ctx.Value(contextKey{}).(*Policy)
	return p, ok && p != nil
}

func WithEscalationGrant(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, escalationGrantKey{}, true)
}

func HasEscalationGrant(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	granted, _ := ctx.Value(escalationGrantKey{}).(bool)
	return granted
}

func (p *Policy) Mode() Mode {
	if p == nil {
		return ModeDangerFullAccess
	}
	return p.mode
}

func (p *Policy) CWD() string {
	if p == nil {
		return ""
	}
	return p.cwd
}

func (p *Policy) WritableRoots() []string {
	if p == nil {
		return nil
	}
	return append([]string(nil), p.writableRoots...)
}

func (p *Policy) Network() NetworkPolicy {
	if p == nil {
		return NetworkPolicy{Enabled: true}
	}
	return p.network
}

func (p *Policy) CheckRead(path string) error {
	if p == nil || p.mode == ModeDangerFullAccess {
		return nil
	}
	normalized, err := normalizePath(path)
	if err != nil {
		return fmt.Errorf("sandbox read denied: %w", err)
	}
	if p.isDenied(normalized) {
		return fmt.Errorf("sandbox read denied for %s: path is denied", normalized)
	}
	return nil
}

func (p *Policy) CheckWrite(path string) error {
	if p == nil || p.mode == ModeDangerFullAccess {
		return nil
	}
	normalized, err := normalizePath(path)
	if err != nil {
		return fmt.Errorf("sandbox write denied: %w", err)
	}
	if p.isDenied(normalized) {
		return fmt.Errorf("sandbox write denied for %s: path is denied", normalized)
	}
	if p.mode == ModeReadOnly {
		return fmt.Errorf("sandbox write denied for %s: mode is read-only", normalized)
	}
	for _, root := range p.writableRoots {
		if pathWithin(normalized, root) {
			return nil
		}
	}
	return fmt.Errorf("sandbox write denied for %s: outside writable roots", normalized)
}

func (p *Policy) CheckNetworkURL(raw string) error {
	if p == nil {
		return nil
	}
	networkPolicy := p.network
	if !networkPolicy.Enabled {
		return fmt.Errorf("sandbox network denied for %s: network is disabled", raw)
	}
	if len(networkPolicy.AllowedDomains) == 0 {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("sandbox network denied: invalid url: %w", err)
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("sandbox network denied for %s: missing host", raw)
	}
	host = normalizeHost(host)
	for _, pattern := range networkPolicy.AllowedDomains {
		if domainMatches(host, pattern) {
			return nil
		}
	}
	return fmt.Errorf("sandbox network denied for %s: host %s is not allowlisted", raw, host)
}

func (p *Policy) isDenied(path string) bool {
	for _, deny := range p.denyPaths {
		if pathWithin(path, deny) {
			return true
		}
	}
	return false
}

func normalizePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			if path == "~" {
				path = home
			} else if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
				path = filepath.Join(home, path[2:])
			}
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func appendUniquePath(paths []string, path string) []string {
	for _, existing := range paths {
		if samePath(existing, path) {
			return paths
		}
	}
	return append(paths, path)
}

func pathWithin(path, root string) bool {
	if samePath(path, root) {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func normalizeHost(host string) string {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if parsed := net.ParseIP(host); parsed != nil {
		return parsed.String()
	}
	return host
}

func domainMatches(host, pattern string) bool {
	pattern = normalizeHost(pattern)
	if pattern == "" {
		return false
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*.")
		return host != suffix && strings.HasSuffix(host, "."+suffix)
	}
	return host == pattern
}
