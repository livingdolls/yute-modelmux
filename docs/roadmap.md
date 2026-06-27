# Roadmap Fitur ModelMux

Tanggal: 2026-06-26

## Ringkasan Kondisi Saat Ini

ModelMux saat ini sudah memiliki fondasi MVP yang kuat:

- Local proxy OpenAI-compatible.
- Endpoint `/v1/chat/completions`.
- Endpoint `/v1/completions`.
- Endpoint `/v1/models`, `/health`, dan `/metrics`.
- Routing model dan model group.
- Rotasi API key dengan strategi failover, round-robin, dan least-error.
- Cooldown otomatis untuk rate limit, timeout, dan server error.
- Streaming response untuk request `stream: true`.
- Manual key test dari CLI dan TUI.
- Config YAML dengan dukungan `value_env`.
- TUI dashboard, chat sessions, logs, keys, dan config editor.
- Auth token lokal opsional.
- Warning saat bind ke `0.0.0.0` tanpa auth.

Roadmap ini berfokus pada fitur produk/teknis yang bisa diimplementasikan berikutnya agar ModelMux lebih siap dipakai harian dan lebih dekat ke produk lengkap.

## Prioritas Fitur

Urutan prioritas yang disarankan:

1. SQLite persistence.
2. Observability production-ready.
3. Budget dan quota policy.
4. CLI management.
5. Provider native adapters.
6. Security secret store.
7. UX operasional lanjutan.

## 1. SQLite Persistence

### Tujuan

Membuat runtime state tidak hilang saat aplikasi restart.

Saat ini status key, usage count, error count, cooldown, dan request logs masih disimpan in-memory. Ini cukup untuk MVP, tetapi kurang ideal untuk penggunaan harian karena semua observability dan state operasional hilang saat service dimatikan.

### Perubahan Config/API

Tambahkan config opsional:

```yaml
storage:
  type: "sqlite"
  path: "~/.local/share/modelmux/modelmux.db"
```

Jika `storage.type` kosong, ModelMux tetap berjalan dengan mode in-memory seperti sekarang.

### Behavior Utama

- Saat router dibuat, config YAML tetap menjadi source of truth untuk provider, model, group, dan key definition.
- Runtime state key dibaca dari SQLite lalu di-merge berdasarkan `key_id`.
- `MarkKeyResult` menulis perubahan ke memory dan SQLite.
- Request logs disimpan ke SQLite jika storage aktif.
- TUI Logs dan `/metrics` membaca state runtime terbaru.
- Mode SQLite tidak boleh memaksa user melakukan migrasi manual untuk config lama.

### Data Minimal

Tabel `keys_runtime`:

```sql
key_id TEXT PRIMARY KEY,
status TEXT,
used_count INTEGER,
error_count INTEGER,
last_used_at TEXT,
cooldown_end TEXT,
updated_at TEXT
```

Tabel `request_logs`:

```sql
id TEXT PRIMARY KEY,
group_id TEXT,
model_id TEXT,
provider_id TEXT,
key_id TEXT,
status_code INTEGER,
error TEXT,
latency_ms INTEGER,
token_input INTEGER,
token_output INTEGER,
created_at TEXT
```

### Test yang Dibutuhkan

- Router restart membaca kembali cooldown dari SQLite.
- `MarkKeyResult` menyimpan `used_count`, `error_count`, status, dan cooldown.
- Request logs tetap tersedia setelah service dibuat ulang.
- Mode tanpa `storage` tetap memakai in-memory dan tidak membuat file DB.

## 2. Observability Production-Ready

### Tujuan

Membuat ModelMux mudah dipantau oleh user dan monitoring system.

Saat ini `/metrics` sudah tersedia dalam format JSON dasar, tetapi belum cukup untuk debugging produksi, monitoring Prometheus, atau analisis performa.

### Perubahan Config/API

Endpoint:

```txt
GET /metrics
GET /metrics?format=json
GET /metrics?format=prometheus
GET /logs
```

CLI:

```bash
modelmux logs
modelmux logs --json
```

### Behavior Utama

- `/metrics` tetap default JSON untuk backward compatibility.
- `format=prometheus` menghasilkan text format Prometheus.
- `/logs` mengembalikan request logs dengan pagination/filter sederhana.
- Semua output harus meredaksi secret.
- Metrics dihitung dari logs dan runtime key state.

### Metrics yang Disarankan

- `requests_total`
- `errors_total`
- `rate_limits_total`
- `latency_avg_ms`
- `latency_p95_ms`
- `active_keys`
- `cooldown_keys`
- `invalid_keys`
- `limited_keys`

Dimensi utama:

- provider
- model
- group
- key

### Test yang Dibutuhkan

- `/metrics?format=json` tetap valid JSON.
- `/metrics?format=prometheus` valid text metrics.
- Metrics tidak mengandung API key.
- `/logs` tidak mengandung API key.
- Latency average dan p95 benar untuk sample logs.

## 3. Budget dan Quota Policy

### Tujuan

Memberi kontrol penggunaan lokal agar ModelMux bisa membatasi key sebelum provider mengembalikan 429.

Fitur ini penting untuk user yang memakai banyak API key dengan batas harian, batas token, atau policy penggunaan internal.

### Perubahan Config/API

Tambahkan field opsional pada key:

```yaml
keys:
  - id: "openai-key-1"
    provider_id: "openai"
    model_id: "gpt-5.5-fast"
    value_env: "OPENAI_KEY_1"
    daily_request_limit: 1000
    daily_token_limit: 500000
```

### Behavior Utama

- Counter harian disimpan di SQLite.
- Jika key mencapai `daily_request_limit`, status runtime menjadi `limited`.
- Jika key mencapai `daily_token_limit`, status runtime menjadi `limited`.
- `SelectKey` melewati key dengan status `limited`.
- Status `limited` reset saat window harian berganti.
- TUI Keys menampilkan usage harian dan limit.
- Metrics menampilkan limited key count dan quota usage.

### Test yang Dibutuhkan

- Key berubah menjadi `limited` saat request count melewati limit.
- Key berubah menjadi `limited` saat token count melewati limit.
- `SelectKey` melewati key limited.
- Reset harian mengaktifkan kembali key.
- Counter bertahan setelah restart jika SQLite aktif.

## 4. CLI Management

### Tujuan

Membuat operasi dasar bisa dilakukan tanpa membuka TUI atau mengedit YAML manual.

Saat ini CLI sudah punya `config init`, `start`, `tui`, dan `key test`. Command operasional lain masih bisa ditambahkan agar ModelMux nyaman dipakai dari script atau terminal biasa.

### Perubahan CLI

Command read-only:

```bash
modelmux providers
modelmux models
modelmux groups
modelmux keys
modelmux logs
```

Command mutasi config:

```bash
modelmux provider add
modelmux model add
modelmux group add
modelmux key add
modelmux key disable --id ...
modelmux key enable --id ...
```

### Behavior Utama

- Output default berupa table.
- Semua command read-only mendukung `--json`.
- Command mutasi memakai validasi config yang sama dengan TUI.
- Command mutasi menyimpan YAML dengan permission aman.
- `key add` sebaiknya mendorong `value_env`, bukan `value`.

### Test yang Dibutuhkan

- CLI read-only menghasilkan table dan JSON valid.
- `key add` membuat config valid.
- `key disable` dan `key enable` mengubah status key.
- Command mutasi gagal jika config menjadi invalid.
- Secret tidak muncul di output read-only.

## 5. Provider Native Adapters

### Tujuan

Mendukung provider yang tidak mengikuti format OpenAI-compatible secara native.

Saat ini ModelMux bekerja paling baik dengan provider yang punya API OpenAI-compatible. Untuk produk lengkap, provider native seperti Anthropic dan Gemini bisa ditambahkan dengan adapter eksplisit.

### Perubahan Config/API

Provider type:

```yaml
providers:
  - id: "anthropic"
    type: "anthropic"
    base_url: "https://api.anthropic.com"
    auth_type: "header"
    auth_header_name: "x-api-key"
```

Nilai yang didukung:

```txt
openai-compatible
anthropic
gemini
```

### Behavior Utama

- OpenAI-compatible tetap default.
- Service memilih provider client berdasarkan `provider.type`.
- Client lokal tetap memanggil ModelMux dengan format OpenAI-compatible.
- Adapter native menerjemahkan request ke format provider.
- Adapter native menerjemahkan response kembali ke format OpenAI-compatible.
- Streaming native dikonversi ke SSE OpenAI-compatible.

### Test yang Dibutuhkan

- Anthropic request mapping.
- Anthropic response mapping.
- Gemini request mapping.
- Gemini response mapping.
- Streaming native dikonversi menjadi SSE OpenAI-compatible.
- Error provider native diklasifikasi ke retry/cooldown dengan benar.

## 6. Security Secret Store

### Tujuan

Mengurangi risiko penyimpanan API key langsung di YAML.

`value_env` sudah jauh lebih aman, tetapi untuk produk lengkap ModelMux bisa menyediakan secret store lokal agar user tidak perlu mengelola env var sendiri.

### Perubahan Config/API

Tambahkan opsi `secret_ref`:

```yaml
keys:
  - id: "openai-key-1"
    provider_id: "openai"
    model_id: "gpt-5.5-fast"
    secret_ref: "keychain:modelmux/openai-key-1"
```

Prioritas secret resolution:

1. `secret_ref`
2. `value_env`
3. `value`

### Behavior Utama

- Gunakan OS keychain jika tersedia.
- Gunakan encrypted local store sebagai fallback.
- Config YAML hanya menyimpan reference.
- CLI/TUI bisa import `value` atau `value_env` ke secret store.
- Secret tidak boleh muncul di logs, metrics, TUI normal view, CLI read-only, atau error response.

### Test yang Dibutuhkan

- Secret resolve dari `secret_ref`.
- Fallback ke `value_env` tetap berjalan.
- Save config tidak menulis secret plaintext.
- Output logs/metrics/CLI/TUI tidak mengandung secret.
- Error handling jelas jika secret ref tidak ditemukan.

## 7. UX Operasional Lanjutan

### Tujuan

Membuat TUI lebih nyaman untuk operasional harian.

### Fitur TUI

- Filter logs berdasarkan provider, model, key, status, dan error.
- Detail panel untuk request log.
- Export logs ke JSON/CSV.
- Bulk key test untuk semua key.
- Health page untuk provider status.
- Persistent chat sessions jika SQLite aktif.
- Warning visual jika server bind ke public host tanpa auth.
- Tampilan quota usage per key.

### Behavior Utama

- Fitur ini tidak mengubah behavior routing utama.
- Semua data sensitif tetap disembunyikan.
- Jika SQLite tidak aktif, fitur persistence chat/log export hanya memakai data in-memory.

### Test yang Dibutuhkan

- Filter logs menghasilkan subset yang benar.
- Bulk key test tidak memblokir UI.
- Export logs tidak mengandung secret.
- Health page menampilkan provider enabled/disabled dan hasil test terakhir.

## Asumsi Baseline Proyek

Roadmap ini mengasumsikan baseline berikut sudah tersedia:

- Streaming sudah stabil dan teruji.
- Manual key test sudah tersedia dari CLI dan TUI.
- Endpoint `/v1/completions` sudah tersedia.
- Auth env validation sudah ada.
- Config `value_env` sudah didukung.
- SQLite tetap opsional, bukan dependency wajib untuk user lama.
- OpenAI-compatible tetap menjadi interface lokal utama untuk client.

## Rekomendasi Urutan Eksekusi

### Phase 1: Persistence dan Observability

1. Tambahkan SQLite persistence.
2. Tambahkan `/logs`.
3. Tambahkan Prometheus metrics.
4. Tambahkan latency avg/p95 dan rate limit count.

### Phase 2: Policy dan CLI

1. Tambahkan quota harian.
2. Tambahkan status `limited` berbasis policy lokal.
3. Tambahkan CLI read-only.
4. Tambahkan CLI mutasi config.

### Phase 3: Provider dan Security

1. Tambahkan provider client registry.
2. Tambahkan Anthropic native adapter.
3. Tambahkan Gemini native adapter.
4. Tambahkan secret store/keychain.

### Phase 4: UX Lanjutan

1. Tambahkan filter logs lanjutan.
2. Tambahkan bulk key test.
3. Tambahkan provider health page.
4. Tambahkan persistent chat sessions.

## Definition of Done Roadmap

Roadmap produk lengkap dianggap tercapai jika:

- State runtime bisa bertahan setelah restart.
- Metrics bisa dipakai oleh manusia dan monitoring system.
- User bisa mengontrol budget/quota lokal.
- Operasi utama bisa dilakukan dari CLI dan TUI.
- Provider native minimal Anthropic dan Gemini tersedia.
- Secret bisa dikelola tanpa plaintext YAML.
- Semua fitur tetap menjaga kompatibilitas config lama.
- Semua fitur utama punya test unit/integration yang relevan.
