# Go EPP HTTP Bridge

Service Go ini membuka port EPP (default `:700`), menerima frame EPP dari registrar, lalu meneruskan request ke backend HTTP seperti implementasi Java.

`config.properties` Java dipakai sebagai **referensi mapping saja**. Runtime Go membaca konfigurasi dari file `.env`.

## Fitur penting
- Parsing frame EPP RFC5734 dengan validasi panjang frame (`EPP_MAX_FRAME_BYTES`).
- TLS frontend opsional + mTLS (`TLS_CLIENT_AUTH=REQUIRE|OPTIONAL|NONE`).
- Auth login registrar ke backend auth, lalu command forward ke backend command.
- Mitigasi flood / DDoS berbasis rate limit multi-dimensi:
  - per IP,
  - per client/username,
  - per channel (koneksi),
  - per tipe command (`read` vs `write`) untuk IP dan client.
- Perlindungan cardinality attack pada rate limiter via batas maksimal key aktif (`EPP_RATELIMIT_MAX_KEYS`).
- Batas koneksi concurrent (`EPP_MAX_CONNS`) agar service tetap stabil saat traffic spike.
- Logging format JSON (default) supaya mudah diolah PM2/log pipeline.

## Konfigurasi `.env`
Default file yang dibaca: `./.env` (bisa diubah via `-env-file` atau `EPP_ENV_FILE`).

Contoh tersedia di `env.example`.

### Mapping referensi dari Java
- `server.port` -> `SERVER_PORT`
- `server.ssl.enabled` -> `SERVER_SSL_ENABLED`
- `authbackend.url` -> `AUTHBACKEND_URL`
- `backend.url` -> `BACKEND_URL`
- `logoutbackend.url` -> `LOGOUTBACKEND_URL`
- `idle.timeout.seconds` -> `IDLE_TIMEOUT_SECONDS`
- `tls.client.auth` -> `TLS_CLIENT_AUTH` (`NONE`/`OPTIONAL`/`REQUIRE`)
- `ratelimit.ip.rules` -> `RATELIMIT_IP_RULES`
- `ratelimit.client.rules` -> `RATELIMIT_CLIENT_RULES`
- `ratelimit.channel.rules` -> `RATELIMIT_CHANNEL_RULES`
- `ratelimit.write.rules` -> `RATELIMIT_WRITE_RULES` (legacy, shared write limiter)
- `ratelimit.read.rules` -> `RATELIMIT_READ_RULES` (legacy, shared read limiter)
- `ratelimit.read.ip.rules` -> `RATELIMIT_READ_IP_RULES`
- `ratelimit.write.ip.rules` -> `RATELIMIT_WRITE_IP_RULES`
- `ratelimit.read.client.rules` -> `RATELIMIT_READ_CLIENT_RULES`
- `ratelimit.write.client.rules` -> `RATELIMIT_WRITE_CLIENT_RULES`

### Variabel tambahan (operasional)
- `EPP_CONNECT_TIMEOUT` (default `5s`) timeout call backend.
- `EPP_BACKEND_TIMEOUT` (default `15s`) timeout total request HTTP ke backend.
- `EPP_BACKEND_DIAL_TIMEOUT` (default `3s`) timeout koneksi TCP ke backend.
- `EPP_BACKEND_TLS_HANDSHAKE_TIMEOUT` (default `3s`) timeout TLS handshake backend.
- `EPP_BACKEND_IDLE_CONN_TIMEOUT` (default `90s`) masa hidup koneksi idle keep-alive.
- `EPP_BACKEND_MAX_IDLE_CONNS` (default `2048`) pool idle koneksi backend global.
- `EPP_BACKEND_MAX_IDLE_CONNS_PER_HOST` (default `1024`) pool idle koneksi backend per host.
- `EPP_BACKEND_MAX_CONNS_PER_HOST` (default `0`) total koneksi backend per host (`0` = unlimited).
- `EPP_WRITE_TIMEOUT` (default `10s`) timeout tulis response ke socket client.
- `EPP_MAX_FRAME_BYTES` (default `65535`) batas ukuran frame EPP.
- `EPP_MAX_CONNS` (default `1000`) batas koneksi concurrent diterima.
- `EPP_RATELIMIT_MAX_KEYS` (default `100000`) batas key unik per scope rate limiter.
- `EPP_LOG_FORMAT` (default `json`) format log: `json` atau `text`.

## Menjalankan lokal
```bash
cd golang/epp-proxy
cp env.example .env
go run .
```

Atau custom env path:
```bash
go run . -env-file /path/to/.env
```

## Build
```bash
go build -o epp-http-bridge .
```

## Menjalankan dengan PM2
1. Build binary:
```bash
go build -o epp-http-bridge .
```
2. Siapkan folder log:
```bash
mkdir -p logs
```
3. Start via ecosystem file:
```bash
pm2 start ecosystem.config.js
```
4. Cek status/log:
```bash
pm2 status
pm2 logs go-epp-proxy
```

`ecosystem.config.js` sudah disediakan dan default memakai `EPP_LOG_FORMAT=json`.

## Catatan hardening DDoS
- Mulai tuning dari `RATELIMIT_IP_RULES` + `RATELIMIT_CHANNEL_RULES` untuk menahan burst awal.
- Pisahkan read/write limit dengan `RATELIMIT_READ_*` dan `RATELIMIT_WRITE_*` agar operasi write lebih ketat.
- Aktifkan mTLS (`SERVER_SSL_ENABLED=true` + `TLS_CLIENT_AUTH=REQUIRE`) agar hanya registrar resmi yang bisa connect.
- Set `EPP_MAX_CONNS` sesuai kapasitas CPU/RAM host.
- Untuk target throughput tinggi, aktifkan keep-alive backend dan tuning pool koneksi (`EPP_BACKEND_MAX_IDLE_CONNS*`) agar tidak terjadi bottleneck saat burst request.
- Pastikan firewall/L4 LB juga punya proteksi SYN flood dan connection limit per source IP.
