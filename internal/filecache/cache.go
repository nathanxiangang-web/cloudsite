// Package filecache owns only a dedicated private one-hour preview cache.
package filecache

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var ErrCache = errors.New("preview cache unavailable")

const Lifetime = time.Hour
const MaxFile = int64(128 << 20)
const MaxTotal = int64(1 << 30)

var keyPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var tempPattern = regexp.MustCompile(`^\.preview-[0-9]+$`)

type Cache struct {
	root string
	slot chan struct{}
}

func New(root string) (*Cache, error) {
	if !filepath.IsAbs(root) {
		return nil, ErrCache
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 {
		return nil, ErrCache
	}
	return &Cache{root: root, slot: make(chan struct{}, 1)}, nil
}
func (c *Cache) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		c.Sweep()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (c *Cache) Sweep() {
	select {
	case c.slot <- struct{}{}:
		defer func() { <-c.slot }()
	default:
		return
	}
	c.sweep()
}
func (c *Cache) sweep() {
	entries, _ := os.ReadDir(c.root)
	for _, e := range entries {
		if !keyPattern.MatchString(e.Name()) && !tempPattern.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err == nil && info.Mode().IsRegular() && time.Since(info.ModTime()) >= Lifetime {
			_ = os.Remove(filepath.Join(c.root, e.Name()))
		}
	}
}
func (c *Cache) Open(ctx context.Context, key string, size int64, load func(context.Context) (io.ReadCloser, error)) (io.ReadSeekCloser, error) {
	if !keyPattern.MatchString(key) || size < 0 || size > MaxFile || load == nil {
		return nil, ErrCache
	}
	select {
	case c.slot <- struct{}{}:
		defer func() { <-c.slot }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.sweep()
	name := filepath.Join(c.root, key)
	if info, err := os.Lstat(name); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			return nil, ErrCache
		}
		if time.Since(info.ModTime()) < Lifetime && info.Size() == size {
			return os.Open(name)
		}
		if os.Remove(name) != nil {
			return nil, ErrCache
		}
	}
	entries, _ := os.ReadDir(c.root)
	var total int64
	for _, e := range entries {
		if keyPattern.MatchString(e.Name()) || tempPattern.MatchString(e.Name()) {
			if info, err := e.Info(); err == nil && info.Mode().IsRegular() {
				total += info.Size()
			}
		}
	}
	if total > MaxTotal-size {
		return nil, ErrCache
	}
	input, err := load(ctx)
	if err != nil {
		return nil, err
	}
	defer input.Close()
	temp, err := os.CreateTemp(c.root, ".preview-")
	if err != nil {
		return nil, ErrCache
	}
	defer func() { temp.Close(); _ = os.Remove(temp.Name()) }()
	written, err := io.Copy(temp, io.LimitReader(input, size+1))
	if err != nil || written != size || ctx.Err() != nil {
		return nil, ErrCache
	}
	if temp.Sync() != nil || temp.Close() != nil || os.Rename(temp.Name(), name) != nil {
		return nil, ErrCache
	}
	return os.Open(name)
}
