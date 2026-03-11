# Go EPP TCP Proxy (HTTP Backend Bridge)

Service Go ini membuka **port TCP EPP** (default `:700`), menerima frame EPP dari registrar, lalu meneruskan request ke backend HTTP.

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


### Penjelasan fungsi setiap setting rate limit

Berikut fungsi dari masing-masing variabel rate limit (dengan baseline pada `env.example`):

- `RATELIMIT_IP_RULES=25/second,1500/minute`
  - Limit global semua transaksi per source IP (read + write).
  - Berfungsi sebagai pagar pertama untuk membatasi burst dari satu IP.
- `RATELIMIT_CLIENT_RULES=25/second,1500/minute`
  - Limit global semua transaksi per username/client ID setelah login.
  - Menjaga fairness antar registrar username walau berada di IP yang sama.
- `RATELIMIT_CHANNEL_RULES=15/second,900/minute`
  - Limit per koneksi TCP aktif (channel).
  - Mencegah satu socket menyedot seluruh kapasitas service.
- `RATELIMIT_WRITE_RULES=40/second,2400/minute`
  - Limit global seluruh transaksi write lintas IP dan username.
  - Dibuat agregat (lebih besar dari limit per IP/per client) agar cukup untuk kombinasi banyak username aktif secara bersamaan.
- `RATELIMIT_READ_RULES=150/second,9000/minute`
  - Limit global seluruh transaksi read lintas IP dan username.
  - Dibuat agregat (lebih besar dari limit per IP/per client) agar tidak memotong trafik saat banyak username membaca paralel.
- `RATELIMIT_WRITE_IP_RULES=5/second,300/minute`
  - Limit write khusus per IP.
  - Sesuai kondisi real: transaksi write per IP dibatasi 5/detik.
- `RATELIMIT_WRITE_CLIENT_RULES=5/second,300/minute`
  - Limit write khusus per username/client.
  - Sesuai kondisi real: transaksi write per username dibatasi 5/detik.
- `RATELIMIT_READ_IP_RULES=20/second,1200/minute`
  - Limit read khusus per IP.
  - Sesuai kondisi real: transaksi read per IP dibatasi 20/detik.
- `RATELIMIT_READ_CLIENT_RULES=20/second,1200/minute`
  - Limit read khusus per username/client.
  - Sesuai kondisi real: transaksi read per username dibatasi 20/detik.

Catatan balancing yang dipakai:
- Pada limit spesifik IP/client, read dibuat ±4x lebih longgar daripada write (20 vs 5 per detik) karena read umumnya lebih ringan dan dominan volumenya.
- Limit global (`RATELIMIT_*_RULES`) sebaiknya lebih besar dari limit spesifik IP/client karena berfungsi sebagai pagu total gabungan banyak username/IP.
- Contoh asumsi kapasitas: 5 username peak + 10 username kecil/sedang menghasilkan kebutuhan kira-kira write 35-40 rps dan read 130-150 rps, sehingga baseline global dipasang di 40 rps (write) dan 150 rps (read).
- Window menit dipakai sebagai guardrail trafik berkelanjutan (bukan hanya burst per detik).

### Variabel tambahan (operasional)
- `EPP_CONNECT_TIMEOUT` (default `5s`) timeout call backend.
- `EPP_BACKEND_TIMEOUT` (default `15s`) timeout total request HTTP ke backend.
- `EPP_BACKEND_DIAL_TIMEOUT` (default `3s`) timeout koneksi TCP ke backend.
- `EPP_BACKEND_TLS_HANDSHAKE_TIMEOUT` (default `3s`) timeout TLS handshake backend.
- `EPP_BACKEND_IDLE_CONN_TIMEOUT` (default `90s`) masa hidup koneksi idle keep-alive.
- `EPP_BACKEND_MAX_IDLE_CONNS` (default `2048`) pool idle koneksi backend global.
- `EPP_BACKEND_MAX_IDLE_CONNS_PER_HOST` (default `1024`) pool idle koneksi backend per host.
- `EPP_BACKEND_MAX_CONNS_PER_HOST` (default `0`) total koneksi backend per host (`0` = unlimited).
- `EPP_BACKEND_RESPONSE_MAX_BYTES` (default `1048576`) batas maksimum ukuran body response backend yang dibaca proxy.
- `EPP_WRITE_TIMEOUT` (default `10s`) timeout tulis response ke socket client.
- `EPP_MAX_FRAME_BYTES` (default `65535`) batas ukuran frame EPP.
- `EPP_MAX_CONNS` (default `1000`) batas koneksi concurrent diterima.
- `EPP_RATELIMIT_MAX_KEYS` (default `100000`) batas key unik per scope rate limiter.
- `EPP_LOG_FORMAT` (default `json`) format log: `json` atau `text`.

## TLS frontend (sertifikat chain)
- `TLS_SERVER_CERT` dapat berisi **full chain** dalam satu file PEM (urutan: leaf certificate lalu intermediate CA). Ini direkomendasikan agar klien dari luar menerima chain lengkap saat handshake.
- `TLS_SERVER_KEY` tetap private key untuk leaf certificate.
- `TLS_CA_CERT` dipakai untuk verifikasi **client certificate** saat `TLS_CLIENT_AUTH` = `OPTIONAL` atau `REQUIRE` (mTLS).
- Jika `TLS_CLIENT_AUTH=NONE`, maka `TLS_CA_CERT` **boleh diabaikan** (tidak dibaca listener).
- Listener dipaksa minimal TLS 1.2.

Contoh membuat full chain:
```bash
cat certs/server.crt certs/intermediate-ca.crt > certs/server-fullchain.pem
```
Lalu set `TLS_SERVER_CERT=certs/server-fullchain.pem`.

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

5. Aktifkan rotasi log harian (sekali setup di host):
```bash
pm2 install pm2-logrotate
pm2 set pm2-logrotate:rotateInterval '0 0 * * *'
pm2 set pm2-logrotate:dateFormat 'YYYY-MM-DD_HH-mm-ss'
pm2 set pm2-logrotate:compress true
pm2 set pm2-logrotate:retain 30
```

Penjelasan singkat:
- `rotateInterval '0 0 * * *'` = rotasi otomatis setiap hari jam 00:00.
- `retain 30` = simpan 30 file hasil rotasi terakhir.
- `compress true` = file log lama dikompres agar hemat disk.
- Cek konfigurasi aktif dengan `pm2 conf pm2-logrotate`.

`ecosystem.config.js` sudah disediakan dan default memakai `EPP_LOG_FORMAT=json`.

## Catatan hardening DDoS
- Mulai tuning dari `RATELIMIT_IP_RULES` + `RATELIMIT_CHANNEL_RULES` untuk menahan burst awal.
- Pisahkan read/write limit dengan `RATELIMIT_READ_*` dan `RATELIMIT_WRITE_*` agar operasi write lebih ketat.
- Aktifkan mTLS (`SERVER_SSL_ENABLED=true` + `TLS_CLIENT_AUTH=REQUIRE`) agar hanya registrar resmi yang bisa connect.
- Set `EPP_MAX_CONNS` sesuai kapasitas CPU/RAM host.
- Untuk target throughput tinggi, aktifkan keep-alive backend dan tuning pool koneksi (`EPP_BACKEND_MAX_IDLE_CONNS*`) agar tidak terjadi bottleneck saat burst request.
- Pastikan firewall/L4 LB juga punya proteksi SYN flood dan connection limit per source IP.

## Internal realtime stats (cara menggunakan)

Fitur ini **internal only** (tidak membuka endpoint HTTP publik). Data yang disimpan:
- koneksi aktif: total, per IP, per username,
- command: total read/write, read/write per IP, read/write per username,
- blocked (rate limit): total, per IP, per username.

Untuk mengambil snapshot realtime, panggil fungsi internal berikut dari kode Go di proses yang sama:

```go
stats := getInternalRealtimeStats(tracker)
```

`tracker` adalah instance `connectionTracker` yang sudah dibuat di `main` (`tracker := newConnectionTracker()`) dan dipakai di `handleConn`.

Contoh bentuk data snapshot (JSON):

```json
{
  "connections": {
    "total": 12,
    "per_ip": {"10.10.10.1": 3},
    "per_username": {"registrarA": 2}
  },
  "commands": {
    "total_read": 1200,
    "total_write": 320,
    "read_per_ip": {"10.10.10.1": 500},
    "write_per_ip": {"10.10.10.1": 80},
    "read_per_username": {"registrarA": 250},
    "write_per_username": {"registrarA": 40}
  },
  "blocked": {
    "total": 15,
    "per_ip": {"10.10.10.9": 12},
    "per_username": {"registrarB": 4}
  }
}
```

Rekomendasi pemakaian:
- panggil berkala dari job internal (misalnya ticker per 5-10 detik),
- kirim ke log/observability internal,
- jangan expose langsung ke jaringan publik.
