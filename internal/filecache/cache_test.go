package filecache

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func cacheForTest(t *testing.T) *Cache {
	t.Helper()
	root := t.TempDir()
	os.Chmod(root, 0700)
	cache, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	return cache
}
func TestOneHourCacheExpiryAndCanonicalKey(t *testing.T) {
	c := cacheForTest(t)
	key := strings.Repeat("a", 64)
	calls := 0
	load := func(context.Context) (io.ReadCloser, error) {
		calls++
		return io.NopCloser(strings.NewReader("data")), nil
	}
	for n := 0; n < 2; n++ {
		file, err := c.Open(context.Background(), key, 4, load)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(file)
		file.Close()
		if string(raw) != "data" {
			t.Fatal("wrong cache body")
		}
	}
	if calls != 1 {
		t.Fatal("cache redownloaded", calls)
	}
	expired := time.Now().Add(-time.Hour - time.Minute)
	os.Chtimes(filepath.Join(c.root, key), expired, expired)
	file, err := c.Open(context.Background(), key, 4, load)
	if err != nil || calls != 2 {
		t.Fatal("expired cache not refreshed", calls, err)
	}
	file.Close()
	if _, err = c.Open(context.Background(), "../../escape", 4, load); err == nil {
		t.Fatal("path traversal")
	}
}
func TestConcurrentMissDownloadsOnceAndSweepOnlyOwnedNames(t *testing.T) {
	c := cacheForTest(t)
	key := strings.Repeat("b", 64)
	var count atomic.Int32
	var group sync.WaitGroup
	for n := 0; n < 12; n++ {
		group.Add(1)
		go func() {
			defer group.Done()
			f, e := c.Open(context.Background(), key, 1, func(context.Context) (io.ReadCloser, error) {
				count.Add(1)
				return io.NopCloser(strings.NewReader("x")), nil
			})
			if e != nil {
				t.Error(e)
				return
			}
			f.Close()
		}()
	}
	group.Wait()
	if count.Load() != 1 {
		t.Fatal(count.Load())
	}
	expired := time.Now().Add(-2 * time.Hour)
	owned := filepath.Join(c.root, key)
	orphan := filepath.Join(c.root, ".preview-123")
	other := filepath.Join(c.root, "operator-note.txt")
	os.WriteFile(orphan, []byte("partial"), 0600)
	os.WriteFile(other, []byte("keep"), 0600)
	for _, file := range []string{owned, orphan, other} {
		os.Chtimes(file, expired, expired)
	}
	c.Sweep()
	for _, file := range []string{owned, orphan} {
		if _, e := os.Stat(file); !os.IsNotExist(e) {
			t.Fatal("expired owned cache remains")
		}
	}
	if _, e := os.Stat(other); e != nil {
		t.Fatal("unrelated file removed")
	}
}
func TestCacheRejectsSymlinksPartialWritesAndCancellation(t *testing.T) {
	c := cacheForTest(t)
	key := strings.Repeat("c", 64)
	target := filepath.Join(t.TempDir(), "original")
	os.WriteFile(target, []byte("secret"), 0600)
	os.Symlink(target, filepath.Join(c.root, key))
	load := func(context.Context) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("x")), nil }
	if _, err := c.Open(context.Background(), key, 1, load); err == nil {
		t.Fatal("symlink followed")
	}
	os.Remove(filepath.Join(c.root, key))
	if _, err := c.Open(context.Background(), key, 4, load); err == nil {
		t.Fatal("partial cache accepted")
	}
	if _, err := os.Stat(filepath.Join(c.root, key)); !os.IsNotExist(err) {
		t.Fatal("partial cache published")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Open(ctx, key, 1, load); err == nil {
		t.Fatal("cancelled read")
	}
	if _, err := c.Open(context.Background(), key, MaxFile+1, load); err == nil {
		t.Fatal("size cap")
	}
}
