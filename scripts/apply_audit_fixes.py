#!/usr/bin/env python3
from __future__ import annotations

from pathlib import Path
import re
import subprocess
import textwrap

ROOT = Path(__file__).resolve().parents[1]


def file_text(path: str) -> str:
    return (ROOT / path).read_text(encoding="utf-8")


def save_text(path: str, content: str) -> None:
    target = ROOT / path
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(content, encoding="utf-8")


def replace_once(path: str, old: str, new: str) -> None:
    text = file_text(path)
    count = text.count(old)
    if count != 1:
        raise RuntimeError(f"{path}: expected one match, found {count}: {old[:100]!r}")
    save_text(path, text.replace(old, new, 1))


def replace_in_function(path: str, signature: str, old: str, new: str, expected: int = 1) -> None:
    text = file_text(path)
    start = text.find(signature)
    if start < 0:
        raise RuntimeError(f"{path}: function not found: {signature}")
    next_func = text.find("\nfunc ", start + len(signature))
    end = len(text) if next_func < 0 else next_func
    segment = text[start:end]
    count = segment.count(old)
    if count != expected:
        raise RuntimeError(f"{path}: {signature}: expected {expected} matches, found {count}: {old!r}")
    segment = segment.replace(old, new, expected)
    save_text(path, text[:start] + segment + text[end:])


def append_once(path: str, marker: str, content: str) -> None:
    text = file_text(path)
    if marker in text:
        raise RuntimeError(f"{path}: marker already present: {marker}")
    save_text(path, text.rstrip() + "\n\n" + content.strip() + "\n")


# Shared path and bind-safety helpers.
save_text(
    "internal/config/paths.go",
    textwrap.dedent(
        r'''
        package config

        import (
            "net"
            "os"
            "path/filepath"
            "strings"
        )

        // ExpandHome expands a leading ~/ while leaving all other paths unchanged.
        func ExpandHome(path string) string {
            if strings.HasPrefix(path, "~/") {
                if home, err := os.UserHomeDir(); err == nil && home != "" {
                    return filepath.Join(home, path[2:])
                }
            }
            return path
        }

        // SecretStorePath keeps the encrypted store beside the configured SQLite database.
        func SecretStorePath(storagePath string) string {
            if storagePath == "" {
                storagePath = defaultStoragePath()
            }
            storagePath = ExpandHome(storagePath)
            return filepath.Join(filepath.Dir(storagePath), "secrets.enc")
        }

        // IsLoopbackHost reports whether a bind host is explicitly local-only.
        // Unknown hostnames are treated as non-loopback so exposure warnings fail safe.
        func IsLoopbackHost(host string) bool {
            host = strings.TrimSpace(host)
            if strings.EqualFold(host, "localhost") {
                return true
            }
            host = strings.TrimPrefix(host, "[")
            host = strings.TrimSuffix(host, "]")
            ip := net.ParseIP(host)
            return ip != nil && ip.IsLoopback()
        }
        '''
    ).lstrip(),
)

save_text(
    "internal/config/paths_test.go",
    textwrap.dedent(
        r'''
        package config

        import (
            "path/filepath"
            "testing"
        )

        func TestSecretStorePath(t *testing.T) {
            custom := filepath.Join("data", "custom.db")
            got := SecretStorePath(custom)
            want := filepath.Join("data", "secrets.enc")
            if got != want {
                t.Fatalf("SecretStorePath(%q) = %q, want %q", custom, got, want)
            }
        }

        func TestIsLoopbackHost(t *testing.T) {
            tests := map[string]bool{
                "127.0.0.1": true,
                "::1":       true,
                "[::1]":     true,
                "localhost": true,
                "0.0.0.0":   false,
                "::":        false,
                "192.168.1.5": false,
                "example.internal": false,
            }
            for host, want := range tests {
                if got := IsLoopbackHost(host); got != want {
                    t.Errorf("IsLoopbackHost(%q) = %v, want %v", host, got, want)
                }
            }
        }
        '''
    ).lstrip(),
)

# Configurable bounded buffering for non-streaming upstream responses.
replace_once(
    "internal/config/config.go",
    'MaxRequestBodyMB   int         `yaml:"max_request_body_mb"`\n',
    'MaxRequestBodyMB   int         `yaml:"max_request_body_mb"`\n\tMaxResponseBodyMB  int         `yaml:"max_response_body_mb"`\n',
)
replace_once(
    "internal/config/config.go",
    'Server:      ServerConfig{Host: "127.0.0.1", Port: 8787, ReadTimeoutSecond: 60, WriteTimeoutSecond: 300, AuthTokenEnv: "MODELMUX_AUTH_TOKEN", MaxRequestBodyMB: 10},',
    'Server:      ServerConfig{Host: "127.0.0.1", Port: 8787, ReadTimeoutSecond: 60, WriteTimeoutSecond: 300, AuthTokenEnv: "MODELMUX_AUTH_TOKEN", MaxRequestBodyMB: 10, MaxResponseBodyMB: 20},',
)
replace_once(
    "internal/config/config.go",
    '''\tif c.Server.MaxRequestBodyMB < 0 {
\t\terrs = append(errs, "server.max_request_body_mb must be non-negative")
\t}
''',
    '''\tif c.Server.MaxRequestBodyMB < 0 {
\t\terrs = append(errs, "server.max_request_body_mb must be non-negative")
\t}
\tif c.Server.MaxResponseBodyMB < 0 {
\t\terrs = append(errs, "server.max_response_body_mb must be non-negative")
\t}
''',
)

# Fix both duplicated secret path implementations and broaden unsafe-bind warning.
replace_once(
    "cmd/modelmux/main.go",
    '''\t\t\tif cfg.Server.Host == "0.0.0.0" && !cfg.Server.RequireAuth {
\t\t\t\tfmt.Fprintln(cmd.ErrOrStderr(), "WARNING: server bound to 0.0.0.0 without authentication enabled.")
\t\t\t\tfmt.Fprintln(cmd.ErrOrStderr(), "Anyone on the network can use your API keys. Set server.require_auth=true and server.auth_token_env.")
\t\t\t}
''',
    '''\t\t\tif !cfg.Server.RequireAuth && !config.IsLoopbackHost(cfg.Server.Host) {
\t\t\t\tfmt.Fprintf(cmd.ErrOrStderr(), "WARNING: server bound to %s without authentication enabled.\\n", cfg.Server.Host)
\t\t\t\tfmt.Fprintln(cmd.ErrOrStderr(), "Anyone able to reach this address can use your API keys. Set server.require_auth=true and server.auth_token_env.")
\t\t\t}
''',
)
replace_once(
    "cmd/modelmux/main.go",
    '''func secretPath(cfg *config.Config) string {
\tpath := cfg.Storage.Path
\tif path == "" {
\t\tpath = config.Default().Storage.Path
\t}
\tpath = expandHome(path)
\tdir := strings.TrimSuffix(path, "modelmux.db")
\tif dir == path {
\t\tdir = path + ".d"
\t}
\treturn dir + "secrets.enc"
}
''',
    '''func secretPath(cfg *config.Config) string {
\treturn config.SecretStorePath(cfg.Storage.Path)
}
''',
)
replace_once(
    "internal/adapter/httpserver/server.go",
    '''func resolveSecretPath(cfg *config.Config) string {
\tdbPath := cfg.Storage.Path
\tif dbPath == "" {
\t\tdbPath = config.Default().Storage.Path
\t}
\tdbPath = expandHome(dbPath)
\tdir := strings.TrimSuffix(dbPath, "modelmux.db")
\tif dir == dbPath {
\t\tdir = dbPath + ".d"
\t}
\treturn dir + "secrets.enc"
}
''',
    '''func resolveSecretPath(cfg *config.Config) string {
\treturn config.SecretStorePath(cfg.Storage.Path)
}
''',
)

# Atomic, validated secret persistence.
replace_once(
    "internal/secret/store.go",
    '''\tdata, err := os.ReadFile(path)
\tif err == nil {
\t\tif err := s.decryptFile(data, masterKey); err != nil {
\t\t\treturn nil, fmt.Errorf("failed to decrypt secret store: %w", err)
\t\t}
\t\treturn s, nil
\t}

\tsalt := make([]byte, saltLen)
''',
    '''\tdata, err := os.ReadFile(path)
\tif err == nil {
\t\tif err := s.decryptFile(data, masterKey); err != nil {
\t\t\treturn nil, fmt.Errorf("failed to decrypt secret store: %w", err)
\t\t}
\t\treturn s, nil
\t}
\tif !errors.Is(err, os.ErrNotExist) {
\t\treturn nil, fmt.Errorf("failed to read secret store: %w", err)
\t}

\tsalt := make([]byte, saltLen)
''',
)
replace_once(
    "internal/secret/store.go",
    '\treturn os.WriteFile(s.path, encrypted, 0o600)\n',
    '\treturn atomicWriteFile(s.path, encrypted, 0o600)\n',
)
replace_once(
    "internal/secret/store.go",
    '''func ImportData(path string, data []byte) error {
\treturn os.WriteFile(path, data, 0o600)
}
''',
    '''func ImportData(path string, data []byte) error {
\tdir := filepath.Dir(path)
\tif err := os.MkdirAll(dir, 0o700); err != nil {
\t\treturn err
\t}

\tcandidate, err := os.CreateTemp(dir, ".modelmux-import-*.enc")
\tif err != nil {
\t\treturn err
\t}
\tcandidatePath := candidate.Name()
\tif err := candidate.Close(); err != nil {
\t\t_ = os.Remove(candidatePath)
\t\treturn err
\t}
\t_ = os.Remove(candidatePath)
\tdefer os.Remove(candidatePath)

\tif err := atomicWriteFile(candidatePath, data, 0o600); err != nil {
\t\treturn err
\t}
\tif _, err := NewStore(candidatePath); err != nil {
\t\treturn fmt.Errorf("invalid secret store import: %w", err)
\t}
\tvalidated, err := os.ReadFile(candidatePath)
\tif err != nil {
\t\treturn err
\t}
\treturn atomicWriteFile(path, validated, 0o600)
}
''',
)
replace_in_function(
    "internal/secret/store.go",
    "func (s *Store) RotateKey",
    '\treturn os.WriteFile(s.path, encrypted, 0o600)\n',
    '\treturn atomicWriteFile(s.path, encrypted, 0o600)\n',
)
append_once(
    "internal/secret/store.go",
    "func atomicWriteFile(",
    r'''
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".modelmux-secret-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	removeTmp = false
	if dirFile, err := os.Open(dir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}
	return nil
}
''',
)
append_once(
    "internal/secret/store_test.go",
    "TestImportRejectsInvalidDataWithoutReplacingExisting",
    r'''
func TestImportRejectsInvalidDataWithoutReplacingExisting(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "secrets.enc")
	t.Setenv("MODELMUX_MASTER_KEY", "import-validation-key")

	store, err := NewStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("existing", "preserved"); err != nil {
		t.Fatal(err)
	}
	if err := ImportData(storePath, []byte("not encrypted data")); err == nil {
		t.Fatal("invalid import should fail")
	}

	reopened, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("existing store was damaged: %v", err)
	}
	if got, _ := reopened.Get("existing"); got != "preserved" {
		t.Fatalf("existing value = %q, want preserved", got)
	}
}

func TestImportRejectsWrongMasterKey(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.enc")
	targetPath := filepath.Join(dir, "target.enc")

	t.Setenv("MODELMUX_MASTER_KEY", "source-master-key")
	source, err := NewStore(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Set("source", "value"); err != nil {
		t.Fatal(err)
	}
	backup, err := source.ExportData()
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("MODELMUX_MASTER_KEY", "target-master-key")
	target, err := NewStore(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Set("target", "preserved"); err != nil {
		t.Fatal(err)
	}
	if err := ImportData(targetPath, backup); err == nil {
		t.Fatal("import encrypted with another master key should fail")
	}

	reopened, err := NewStore(targetPath)
	if err != nil {
		t.Fatalf("target store was damaged: %v", err)
	}
	if got, _ := reopened.Get("target"); got != "preserved" {
		t.Fatalf("target value = %q, want preserved", got)
	}
}
''',
)

# Persistent storage must fail closed after corruption or a write failure.
replace_once(
    "internal/storage/storage.go",
    '\t"strings"\n\t"time"\n',
    '\t"strings"\n\t"sync"\n\t"time"\n',
)
replace_once(
    "internal/storage/storage.go",
    '''\tif err := migrate(db); err != nil {
\t\tdb.Close()
\t\treturn nil, fmt.Errorf("storage: migrate: %w", err)
\t}

\treturn &sqliteStore{db: db}, nil
}

 type sqliteStore struct {
\tdb *sql.DB
}
'''.replace("\n type", "\ntype"),
    '''\tif err := migrate(db); err != nil {
\t\tdb.Close()
\t\treturn nil, fmt.Errorf("storage: migrate: %w", err)
\t}

\tvar integrity string
\tif err := db.QueryRow("PRAGMA quick_check").Scan(&integrity); err != nil || integrity != "ok" {
\t\tdb.Close()
\t\tif err != nil {
\t\t\treturn nil, fmt.Errorf("storage: integrity check: %w", err)
\t\t}
\t\treturn nil, fmt.Errorf("storage: integrity check failed: %s", integrity)
\t}

\tstore := &sqliteStore{db: db}
\tif _, err := store.LoadKeyRuntime(); err != nil {
\t\tdb.Close()
\t\treturn nil, fmt.Errorf("storage: load key runtime: %w", err)
\t}
\tif _, err := store.LoadRequestLogs(); err != nil {
\t\tdb.Close()
\t\treturn nil, fmt.Errorf("storage: load request logs: %w", err)
\t}
\treturn store, nil
}

type HealthReporter interface {
\tHealth() error
}

func HealthError(store Storage) error {
\tif store == nil {
\t\treturn nil
\t}
\treporter, ok := store.(HealthReporter)
\tif !ok {
\t\treturn nil
\t}
\treturn reporter.Health()
}

type sqliteStore struct {
\tdb *sql.DB
\thealthMu sync.RWMutex
\thealthErr error
}
''',
)
replace_once(
    "internal/storage/storage.go",
    '''func (s *sqliteStore) Close() error {
\treturn s.db.Close()
}
''',
    '''func (s *sqliteStore) Close() error {
\treturn s.db.Close()
}

func (s *sqliteStore) Health() error {
\ts.healthMu.RLock()
\tdefer s.healthMu.RUnlock()
\treturn s.healthErr
}

func (s *sqliteStore) markUnhealthy(err error) error {
\tif err == nil {
\t\treturn nil
\t}
\ts.healthMu.Lock()
\tif s.healthErr == nil {
\t\ts.healthErr = err
\t}
\ts.healthMu.Unlock()
\treturn err
}
''',
)
for signature in [
    "func (s *sqliteStore) SaveKeyRuntime",
    "func (s *sqliteStore) SaveRequestLog",
    "func (s *sqliteStore) SaveRouteTrace",
    "func (s *sqliteStore) SaveChatMessage",
    "func (s *sqliteStore) SaveEvalRun",
    "func (s *sqliteStore) SaveEvalResult",
    "func (s *sqliteStore) Vacuum",
]:
    replace_in_function("internal/storage/storage.go", signature, "\treturn err\n", "\treturn s.markUnhealthy(err)\n")
replace_in_function(
    "internal/storage/storage.go",
    "func (s *sqliteStore) SaveChatSession",
    "\t\treturn 0, err\n",
    "\t\treturn 0, s.markUnhealthy(err)\n",
)
replace_in_function(
    "internal/storage/storage.go",
    "func (s *sqliteStore) SaveChatSession",
    "\treturn int(id), err\n",
    "\treturn int(id), s.markUnhealthy(err)\n",
)
replace_in_function(
    "internal/storage/storage.go",
    "func (s *sqliteStore) PruneBefore",
    '\t\treturn total, fmt.Errorf("prune errors: %s", strings.Join(errs, "; "))\n',
    '\t\treturn total, s.markUnhealthy(fmt.Errorf("prune errors: %s", strings.Join(errs, "; ")))\n',
)
replace_in_function(
    "internal/storage/storage.go",
    "func (s *sqliteStore) QueryRequestLogs",
    "\t\treturn nil, 0, err\n",
    "\t\treturn nil, 0, s.markUnhealthy(err)\n",
    expected=2,
)

save_text(
    "internal/storage/health_test.go",
    textwrap.dedent(
        r'''
        package storage

        import (
            "path/filepath"
            "testing"
        )

        func TestStorageWriteFailureMarksUnhealthy(t *testing.T) {
            store, err := New(filepath.Join(t.TempDir(), "modelmux.db"))
            if err != nil {
                t.Fatal(err)
            }
            if err := store.Close(); err != nil {
                t.Fatal(err)
            }
            if err := store.SaveRequestLog(RequestLogRecord{ID: "after-close"}); err == nil {
                t.Fatal("write after close should fail")
            }
            if err := HealthError(store); err == nil {
                t.Fatal("storage should remain unhealthy after a write failure")
            }
        }

        func TestHealthErrorNilStorage(t *testing.T) {
            if err := HealthError(nil); err != nil {
                t.Fatalf("HealthError(nil) = %v", err)
            }
        }
        '''
    ).lstrip(),
)

# Block subsequent traffic when persistence is unhealthy; expose degraded health.
replace_once(
    "internal/adapter/httpserver/server.go",
    '''\t\trs, release, ok := gen.acquire()
\t\ts.genMu.RUnlock()
''',
    '''\t\tif err := storage.HealthError(gen.store); err != nil {
\t\t\ts.genMu.RUnlock()
\t\t\twriteJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistent storage unavailable"})
\t\t\treturn
\t\t}
\t\trs, release, ok := gen.acquire()
\t\ts.genMu.RUnlock()
''',
)
replace_once(
    "internal/adapter/httpserver/handlers.go",
    '''func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
\twriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
''',
    '''func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
\tif err := storagepkg.HealthError(s.loadStore()); err != nil {
\t\twriteJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "degraded", "error": "persistent storage unavailable"})
\t\treturn
\t}
\twriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
''',
)
replace_once(
    "internal/adapter/httpserver/handlers.go",
    'writeJSON(w, http.StatusBadGateway, map[string]any{"error": map[string]any{"message": err.Error(), "type": "modelmux_error"}})',
    'writeJSON(w, http.StatusBadGateway, map[string]any{"error": map[string]any{"message": "upstream request failed", "type": "modelmux_error"}})',
)

# Bound non-streaming response buffering and handle read failures explicitly.
replace_once(
    "internal/app/service/router_service.go",
    '''\t\tif result.Success && !isStreamRequest(bodyBytes) && resp != nil {
\t\t\trespBodyBytes, readErr := io.ReadAll(resp.Body)
\t\t\tresp.Body.Close()
\t\t\tif readErr == nil {
\t\t\t\tresult.TokenInput, result.TokenOutput = parseTokenUsage(respBodyBytes)
\t\t\t\tresp.Body = io.NopCloser(bytes.NewReader(respBodyBytes))
\t\t\t}
\t\t}
''',
    '''\t\tif result.Success && !isStreamRequest(bodyBytes) && resp != nil {
\t\t\tmaxResponseBytes := int64(s.cfg.Server.MaxResponseBodyMB) * 1024 * 1024
\t\t\tif maxResponseBytes <= 0 {
\t\t\t\tmaxResponseBytes = 20 * 1024 * 1024
\t\t\t}
\t\t\trespBodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
\t\t\t_ = resp.Body.Close()
\t\t\tswitch {
\t\t\tcase readErr != nil:
\t\t\t\terr = fmt.Errorf("read upstream response: %w", readErr)
\t\t\t\tresult.Success = false
\t\t\t\tresult.ShouldRotateKey = true
\t\t\t\tresult.Error = "failed to read upstream response"
\t\t\t\tresult.CooldownSeconds = s.cfg.Cooldown.TimeoutSeconds
\t\t\t\tresp = nil
\t\t\tcase int64(len(respBodyBytes)) > maxResponseBytes:
\t\t\t\terr = &ProxyError{
\t\t\t\t\tHTTPStatus: http.StatusBadGateway,
\t\t\t\t\tType:       "modelmux_upstream_response_too_large",
\t\t\t\t\tCode:       "upstream_response_too_large",
\t\t\t\t\tMessage:    "upstream response exceeded the configured size limit",
\t\t\t\t}
\t\t\t\tresult.Success = false
\t\t\t\tresult.ShouldRotateKey = false
\t\t\t\tresult.Error = "upstream response exceeded the configured size limit"
\t\t\t\tresp = nil
\t\t\tdefault:
\t\t\t\tresult.TokenInput, result.TokenOutput = parseTokenUsage(respBodyBytes)
\t\t\t\tresp.Body = io.NopCloser(bytes.NewReader(respBodyBytes))
\t\t\t}
\t\t}
''',
)

# Document the new response limit wherever the request limit is shown in YAML examples.
for candidate in ROOT.rglob("*.yaml"):
    text = candidate.read_text(encoding="utf-8")
    if "max_request_body_mb:" in text and "max_response_body_mb:" not in text:
        text = re.sub(
            r"^(\s*)max_request_body_mb:\s*([^\n]+)$",
            r"\1max_request_body_mb: \2\n\1max_response_body_mb: 20",
            text,
            count=1,
            flags=re.MULTILINE,
        )
        candidate.write_text(text, encoding="utf-8")

# Enforce unit, race, E2E, vet, and website checks on PRs and main.
save_text(
    ".github/workflows/ci.yml",
    textwrap.dedent(
        r'''
        name: CI

        on:
          pull_request:
          push:
            branches: [main]

        permissions:
          contents: read

        concurrency:
          group: ci-${{ github.workflow }}-${{ github.ref }}
          cancel-in-progress: true

        jobs:
          go:
            runs-on: ubuntu-latest
            steps:
              - uses: actions/checkout@v4
              - uses: actions/setup-go@v5
                with:
                  go-version-file: go.mod
                  cache: true
              - name: Vet
                run: go vet ./...
              - name: Unit and race tests
                run: go test -race ./...
              - name: HTTP E2E tests
                run: go test -race -tags=e2e ./internal/e2e/ -timeout 60s

          website:
            runs-on: ubuntu-latest
            defaults:
              run:
                working-directory: website
            steps:
              - uses: actions/checkout@v4
              - uses: actions/setup-node@v4
                with:
                  node-version: 22
                  cache: npm
                  cache-dependency-path: website/package-lock.json
              - run: npm ci
              - run: npm run typecheck
              - run: npm run build
        '''
    ).lstrip(),
)
replace_once(
    ".github/workflows/release.yml",
    '''      - name: Run tests
        run: go test ./...
''',
    '''      - name: Run checks
        run: |
          go vet ./...
          go test -race ./...
          go test -race -tags=e2e ./internal/e2e/ -timeout 60s
''',
)

# Generate the lockfile from the pinned package manifest.
subprocess.run(
    ["npm", "install", "--package-lock-only", "--ignore-scripts"],
    cwd=ROOT / "website",
    check=True,
)

# Remove the one-shot bootstrap so it cannot run again.
(ROOT / "scripts/apply_audit_fixes.py").unlink()
(ROOT / ".github/workflows/apply-audit-fixes.yml").unlink()
