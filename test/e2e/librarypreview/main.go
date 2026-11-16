// Command librarypreview serves only synthetic Issue187 data on loopback.
// It is not linked into any product command or deployment image.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/integrations/indexcore"
	"github.com/nathanxiangang-web/panta/internal/libraryview"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/producthttp"
	"github.com/nathanxiangang-web/panta/internal/resourceview"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"github.com/nathanxiangang-web/panta/internal/store/postgres"
)

const bindingID = storage.BindingID("98000000-0000-4000-8000-000000000001")

type metrics struct{ Q6, Q7, Q8, Q9, Batch, Single, Pages, Legacy atomic.Int64 }

func (m *metrics) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.Contains(d.SQL, "FROM copies") {
		if strings.Contains(d.SQL, "ANY(") {
			m.Batch.Add(1)
		} else if strings.Contains(d.SQL, "indexcore_resource_id = $2") {
			m.Single.Add(1)
		}
	}
	return ctx
}
func (m *metrics) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func (m *metrics) view() map[string]int64 {
	return map[string]int64{"source_q6": m.Q6.Load(), "source_q7": m.Q7.Load(), "source_q8": m.Q8.Load(), "source_q9": m.Q9.Load(), "copy_batch_queries": m.Batch.Load(), "copy_single_queries": m.Single.Load(), "library_requests": m.Pages.Load(), "legacy_resources_requests": m.Legacy.Load()}
}
func main() {
	if err := run(); err != nil {
		log.Print("isolated library preview failed")
		os.Exit(1)
	}
}
func run() error {
	raw := os.Getenv("PANTA_LIBRARY_PREVIEW_DATABASE_URL")
	u, err := url.Parse(raw)
	if err != nil || u.Host != "127.0.0.1:55445" || u.Path != "/panta_issue187_preview_test" {
		return errors.New("wrong isolated database")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	m := &metrics{}
	cfg, err := pgxpool.ParseConfig(raw)
	if err != nil {
		return err
	}
	cfg.ConnConfig.Tracer = m
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	var version int
	if err = pool.QueryRow(ctx, `SELECT current_setting('server_version_num')::int`).Scan(&version); err != nil || version < 180000 || version >= 190000 {
		return errors.New("PG18 required")
	}
	migrator, _ := postgres.NewMigrator(pool)
	if _, err = migrator.Apply(ctx); err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `INSERT INTO storage_connections(storage_connection_id,provider_type,status,created_at,updated_at) VALUES('98000000-0000-4000-8000-000000000010','FIXTURE','ACTIVE',now(),now()) ON CONFLICT DO NOTHING;
 INSERT INTO storage_bindings(storage_binding_id,storage_connection_id,openlist_mount_path,indexcore_root_id,status,created_at,updated_at) VALUES('98000000-0000-4000-8000-000000000001','98000000-0000-4000-8000-000000000010','/fixture','fixture-root','ACTIVE',now(),now()) ON CONFLICT DO NOTHING;`)
	if err != nil {
		return err
	}
	authRepo, _ := postgres.NewAuthRepository(pool)
	accounts, _ := auth.NewService(authRepo)
	_, err = accounts.Provision(ctx, "fixture", "fixture-only-187", auth.RoleUser)
	if err != nil && !errors.Is(err, auth.ErrConflict) {
		return err
	}
	var dataMu sync.RWMutex
	journal := []map[string]any{}
	removed := map[string]map[string]any{}
	data := []map[string]any{}
	for n := 0; n < 858; n++ {
		ext := "zip"
		if n < 235 {
			ext = "mp4"
		} else if n < 277 {
			ext = "apk"
		} else if n < 283 {
			ext = "pdf"
		}
		name := fmt.Sprintf("fixture-%04d.%s", n, ext)
		dir := n >= 343
		if dir {
			name = fmt.Sprintf("folder-%04d", n)
		}
		data = append(data, map[string]any{"resource_id": fmt.Sprintf("98000000-0000-4000-8000-%012d", n+1), "root_id": "fixture-root", "canonical_path": "/" + name, "name": name, "is_dir": dir, "size": int64(1024 + n), "resource_presence": "PRESENT", "introduced_at_generation": 1, "last_confirmed_generation": 1})
	}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		dataMu.RLock()
		defer dataMu.RUnlock()
		if strings.HasPrefix(r.URL.Path, "/v1/resources/") {
			m.Q7.Add(1)
			id := strings.TrimPrefix(r.URL.Path, "/v1/resources/")
			for _, row := range data {
				if row["resource_id"] == id {
					_ = json.NewEncoder(w).Encode(row)
					return
				}
			}
			if row, ok := removed[id]; ok && r.URL.Query().Get("include_removed") == "true" {
				_ = json.NewEncoder(w).Encode(row)
				return
			}
			w.WriteHeader(404)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "not_found", "message": "synthetic missing"})
			return
		}
		switch r.URL.Path {
		case "/v1/roots/fixture-root/active":
			m.Q6.Add(1)
			time.Sleep(20 * time.Millisecond)
			offset := 0
			if v := r.URL.Query().Get("cursor"); v != "" {
				offset, _ = strconv.Atoi(v)
			}
			if offset < 0 || offset > len(data) {
				w.WriteHeader(400)
				return
			}
			end := min(offset+50, len(data))
			next := ""
			if end < len(data) {
				next = strconv.Itoa(end)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": data[offset:end], "next_cursor": next})
		case "/v1/roots/fixture-root/journal":
			m.Q8.Add(1)
			after, _ := strconv.Atoi(r.URL.Query().Get("after_seq"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if limit <= 0 {
				limit = 100
			}
			items := []any{}
			for _, event := range journal {
				if event["event_seq"].(int) > after && len(items) < limit {
					items = append(items, event)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
		case "/v1/roots/fixture-root/status":
			m.Q9.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"root_id": "fixture-root", "lifecycle_state": "ACTIVE", "current_generation": len(journal) + 1})
		default:
			w.WriteHeader(404)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "not_found", "message": "synthetic missing"})
		}
	}))
	defer fake.Close()
	index, _ := indexcore.NewClient(fake.URL)
	topology, _ := postgres.NewStorageRepository(pool)
	catalog, _ := postgres.NewCatalogRepository(pool)
	tasks, _ := postgres.NewProductReadRepository(pool)
	physical, _ := resourceview.NewPhysicalService(index, catalog)
	reads, _ := product.NewReadService(tasks, topology, index, physical, catalog, []storage.BindingID{bindingID})
	engine, _ := libraryview.New(index, topology, postgres.NewLibrarySnapshotRepository(pool), []storage.BindingID{bindingID})
	if err = engine.Restore(ctx); err != nil {
		return err
	}
	if err = engine.Sync(ctx, bindingID); err != nil {
		return err
	}
	service, _ := product.NewLibraryService(reads, engine, []storage.BindingID{bindingID}, "", "")
	handler, err := producthttp.NewWorkspaceHandler(accounts, "http://127.0.0.1:8187", reads, []producthttp.DisplayBinding{{ID: bindingID, Label: "Isolated synthetic library"}}, postgres.CurrentSchemaVersion, producthttp.WithLibraryScope([]storage.BindingID{bindingID}, true), producthttp.WithLibraryReads(service))
	if err != nil {
		return err
	}
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fixture/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<h1>Issue187 隔离模拟环境</h1><p>只修改本环境的模拟文件，不连接真实网盘。</p><form action="/fixture/add" method="post"><button>新增一个模拟文件</button></form><form action="/fixture/remove" method="post"><button>移除新增的模拟文件</button></form><a href="/">返回文件库</a>`))
			return
		}
		if (r.URL.Path == "/fixture/add" || r.URL.Path == "/fixture/remove") && r.Method == http.MethodPost {
			const id = "98000000-0000-4000-8000-000000001000"
			dataMu.Lock()
			found := -1
			for i, row := range data {
				if row["resource_id"] == id {
					found = i
				}
			}
			eventType := ""
			if r.URL.Path == "/fixture/add" && found < 0 {
				data = append(data, map[string]any{"resource_id": id, "root_id": "fixture-root", "canonical_path": "/fixture-late.pdf", "name": "fixture-late.pdf", "is_dir": false, "size": int64(2048), "resource_presence": "PRESENT", "introduced_at_generation": 2, "last_confirmed_generation": 2})
				delete(removed, id)
				eventType = "resource-added"
			}
			if r.URL.Path == "/fixture/remove" && found >= 0 {
				row := map[string]any{}
				for k, v := range data[found] {
					row[k] = v
				}
				row["resource_presence"] = "REMOVED"
				removed[id] = row
				data = append(data[:found], data[found+1:]...)
				eventType = "resource-removed"
			}
			if eventType != "" {
				journal = append(journal, map[string]any{"event_seq": len(journal) + 1, "generation_number": len(journal) + 2, "intra_generation_seq": 0, "event_type": eventType, "resource_id": id, "payload": "", "committed_at": time.Now().UTC()})
			}
			dataMu.Unlock()
			if err := engine.Sync(ctx, bindingID); err != nil {
				http.Error(w, "synthetic update pending", 503)
				return
			}
			http.Redirect(w, r, "/fixture/", 303)
			return
		}
		if r.URL.Path == "/fixture/metrics" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(m.view())
			return
		}
		if r.URL.Path == "/api/v1/library" {
			m.Pages.Add(1)
		}
		if r.URL.Path == "/api/v1/resources" {
			m.Legacy.Add(1)
		}
		handler.ServeHTTP(w, r)
	})
	server := &http.Server{Addr: "127.0.0.1:18187", Handler: api, ReadHeaderTimeout: 5 * time.Second}
	done := make(chan struct{})
	go func() { defer close(done); engine.Run(ctx) }()
	defer func() { stop(); <-done }()
	if p := os.Getenv("PANTA_LIBRARY_PREVIEW_PIDFILE"); p != "" {
		if err = os.WriteFile(p, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			return err
		}
	}
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shut)
	}()
	log.Print("isolated synthetic library ready on loopback")
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
