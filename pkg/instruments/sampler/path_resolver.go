package sampler

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

type resolveCache struct {
	mu      sync.RWMutex
	entries map[string]string
	hits    atomic.Uint64
	misses  atomic.Uint64
}

type PathResolver struct {
	basePath     string
	rootBasePath string
	cache        *resolveCache
	isWindows    bool
}

func NewPathResolver(basePath string) *PathResolver {
	base := filepath.Clean(basePath)
	if abs, err := filepath.Abs(base); err == nil {
		base = abs
	}
	return &PathResolver{
		basePath:     base,
		rootBasePath: base,
		cache:        &resolveCache{entries: make(map[string]string)},
		isWindows:    runtime.GOOS == "windows",
	}
}

func (r *PathResolver) ForDir(dir string) *PathResolver {
	base := filepath.Clean(dir)
	if abs, err := filepath.Abs(base); err == nil {
		base = abs
	}
	root := r.rootBasePath
	if root == "" {
		root = r.basePath
	}
	return &PathResolver{
		basePath:     base,
		rootBasePath: root,
		cache:        r.cache,
		isWindows:    r.isWindows,
	}
}

func SanitizePath(p string) string {
	p = strings.TrimSpace(p)
	if len(p) >= 2 {
		if (p[0] == '"' && p[len(p)-1] == '"') ||
			(p[0] == '\'' && p[len(p)-1] == '\'') {
			p = p[1 : len(p)-1]
		}
	}
	p = strings.ReplaceAll(p, "\\", "/")
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	p = strings.TrimSpace(p)
	if len(p) >= 2 && p[1] == ':' && ((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z')) {
		p = p[2:]
		p = strings.TrimLeft(p, "/")
	}
	return p
}

func isWithinBase(base, target string) bool {
	realBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		realBase = filepath.Clean(base)
	}
	if abs, err := filepath.Abs(realBase); err == nil {
		realBase = abs
	}
	t := filepath.Clean(target)
	if !filepath.IsAbs(t) {
		t = filepath.Join(realBase, t)
	}
	if abs, err := filepath.Abs(t); err == nil {
		t = abs
	}
	for {
		if realT, err := filepath.EvalSymlinks(t); err == nil {
			t = realT
			break
		}
		parent := filepath.Dir(t)
		if parent == t || parent == "." || parent == "/" {
			break
		}
		t = parent
	}
	if abs, err := filepath.Abs(t); err == nil {
		t = abs
	}
	rel, err := filepath.Rel(realBase, t)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func (r *PathResolver) Resolve(relativePath string) (string, error) {
	sanitized := SanitizePath(relativePath)
	if sanitized == "" {
		return "", &os.PathError{Op: "resolve", Path: relativePath, Err: os.ErrNotExist}
	}
	base := filepath.Clean(r.basePath)
	if absBase, err := filepath.Abs(base); err == nil {
		base = absBase
	}
	root := r.rootBasePath
	if root == "" {
		root = base
	}
	if filepath.IsAbs(filepath.FromSlash(sanitized)) {
		abs := filepath.Clean(filepath.FromSlash(sanitized))
		if absTarget, err := filepath.Abs(abs); err == nil {
			abs = absTarget
		}
		if !isWithinBase(root, abs) {
			return "", &os.PathError{Op: "resolve", Path: relativePath, Err: os.ErrNotExist}
		}
		if _, err := os.Stat(abs); err == nil {
			return abs, nil
		}
		return "", &os.PathError{Op: "resolve", Path: relativePath, Err: os.ErrNotExist}
	}
	absPath := filepath.Clean(filepath.Join(base, filepath.FromSlash(sanitized)))
	if !isWithinBase(root, absPath) {
		return "", &os.PathError{Op: "resolve", Path: relativePath, Err: os.ErrNotExist}
	}

	if _, err := os.Stat(absPath); err == nil {
		return absPath, nil
	}

	if r.isWindows {
		return "", &os.PathError{Op: "resolve", Path: relativePath, Err: os.ErrNotExist}
	}

	return r.resolveCaseInsensitive(sanitized)
}

func (r *PathResolver) resolveCaseInsensitive(path string) (string, error) {
	base := filepath.Clean(r.basePath)
	if absBase, err := filepath.Abs(base); err == nil {
		base = absBase
	}
	root := r.rootBasePath
	if root == "" {
		root = base
	}
	key := strings.ToLower(filepath.Clean(filepath.Join(base, filepath.FromSlash(path))))
	r.cache.mu.RLock()
	if cached, ok := r.cache.entries[key]; ok {
		r.cache.mu.RUnlock()
		if !isWithinBase(root, cached) {
			return "", &os.PathError{Op: "resolve", Path: path, Err: os.ErrNotExist}
		}
		r.cache.hits.Add(1)
		return cached, nil
	}
	r.cache.mu.RUnlock()

	parts := strings.Split(path, "/")
	current := base
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			next := filepath.Dir(current)
			if !isWithinBase(root, next) {
				return "", &os.PathError{Op: "resolve", Path: path, Err: os.ErrNotExist}
			}
			current = next
			continue
		}
		entries, err := os.ReadDir(current)
		if err != nil {
			return "", err
		}
		target := strings.ToLower(part)
		found := false
		for _, entry := range entries {
			if strings.ToLower(entry.Name()) == target {
				next := filepath.Join(current, entry.Name())
				if !isWithinBase(root, next) {
					return "", &os.PathError{Op: "resolve", Path: path, Err: os.ErrNotExist}
				}
				current = next
				found = true
				break
			}
		}
		if !found {
			return "", &os.PathError{Op: "resolve", Path: path, Err: os.ErrNotExist}
		}
	}

	if !isWithinBase(root, current) {
		return "", &os.PathError{Op: "resolve", Path: path, Err: os.ErrNotExist}
	}

	r.cache.mu.Lock()
	r.cache.entries[key] = current
	r.cache.misses.Add(1)
	r.cache.mu.Unlock()
	return current, nil
}

func (r *PathResolver) Hits() uint64 {
	return r.cache.hits.Load()
}

func (r *PathResolver) Misses() uint64 {
	return r.cache.misses.Load()
}
