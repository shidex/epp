# Go EPP HTTP Bridge

Service Go ini membuka port EPP (default `:700`), menerima frame EPP dari registrar, lalu memprosesnya ke backend HTTP yang sama seperti implementasi Java:

- Auth login -> `authRegistrar` (HTTP POST JSON)
- Command EPP -> `processepp` (HTTP POST XML + header `authentication`)

Dengan alur ini backend Java existing tidak perlu diubah.

## Fitur
- Listener EPP TCP/TLS
- Kirim EPP greeting saat koneksi terbuka
- Login flow mengikuti Java (auth backend + ambil `eppSessionToken`)
- Forward command ke backend HTTP dengan header `authentication`
- Response EPP login/logout/error dasar
- Rate limiting per IP/username

## Menjalankan
Dari folder `golang/epp-proxy`:

```bash
cd golang/epp-proxy
go run . \
  -listen :700 \
  -auth-url http://localhost:8080/PANDI-REGISTRAR-0.1/authRegistrar/ \
  -command-url http://localhost:8080/PANDI-CORE-0.1/processepp/
```

Atau via environment variable:

```bash
cd golang/epp-proxy && \
EPP_LISTEN_ADDR=:700 \
EPP_AUTH_URL=http://localhost:8080/PANDI-REGISTRAR-0.1/authRegistrar/ \
EPP_COMMAND_URL=http://localhost:8080/PANDI-CORE-0.1/processepp/ \
go run .
```

## Opsi konfigurasi
- `-listen` / `EPP_LISTEN_ADDR` (default `:700`)
- `-auth-url` / `EPP_AUTH_URL` (default `http://localhost:8080/PANDI-REGISTRAR-0.1/authRegistrar/`)
- `-command-url` / `EPP_COMMAND_URL` (default `http://localhost:8080/PANDI-CORE-0.1/processepp/`)
- `-connect-timeout` / `EPP_CONNECT_TIMEOUT` (default `5s`)
- `-idle-timeout` / `EPP_IDLE_TIMEOUT` (default `10m`)
- `-frontend-tls` / `EPP_FRONTEND_TLS` (default `false`)
- `-frontend-cert` / `EPP_FRONTEND_CERT` (default `certs/server.crt`)
- `-frontend-key` / `EPP_FRONTEND_KEY` (default `certs/server.key`)
- `-rate-limit-max` / `EPP_RATE_LIMIT_MAX` (default `10`)
- `-rate-limit-window` / `EPP_RATE_LIMIT_WINDOW` (default `1m`)
- `-rate-limit-by` / `EPP_RATE_LIMIT_BY` (default `ip_or_username`, opsi: `ip`, `username`, `ip_or_username`)

## Build binary
```bash
go build -o epp-http-bridge .
```
