# Go EPP HTTP Bridge

Service Go ini membuka port EPP (default `:700`), menerima frame EPP dari registrar, lalu meneruskan request ke backend HTTP seperti implementasi Java.

`config.properties` Java dipakai sebagai **referensi mapping saja**. Runtime Go membaca konfigurasi dari file `.env`.

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
- `ratelimit.write.rules` -> `RATELIMIT_WRITE_RULES` (default: `10/second` untuk operasi write seperti `create/update/renew/delete/transfer` pada domain/host/contact)
- `ratelimit.read.rules` -> `RATELIMIT_READ_RULES` (default: `50/second,500/minute` untuk operasi read seperti `login/logout/check/info`)

## Menjalankan
```bash
cd golang/epp-proxy
cp env.example .env
go run .
```

Atau custom env path:
```bash
go run . -env-file /path/to/.env
```

## Opsi penting
- `-env-file` / `EPP_ENV_FILE`
- `-listen`
- `-frontend-tls`, `-frontend-cert`, `-frontend-key`, `-frontend-ca`, `-tls-client-auth`
- `-auth-url`, `-command-url`, `-logout-url`
- `-idle-timeout`, `-connect-timeout`
- `-rate-limit-ip`, `-rate-limit-client`, `-rate-limit-channel`
- `-rate-limit-write`, `-rate-limit-read`
- `-max-frame-size`

## Build
```bash
go build -o epp-http-bridge .
```
