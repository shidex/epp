# Go EPP TCP Forwarder

Service ini adalah versi Go yang membuka port `700` (default), menerima koneksi EPP, lalu melakukan forwarding byte-stream ke backend.

## Fitur
- Default listen di `:700`
- Forward raw TCP (cocok untuk frame EPP 4-byte prefix maupun delimiter XML)
- Opsi TLS di sisi frontend (listener)
- Opsi TLS di sisi backend (upstream)
- Graceful shutdown via SIGINT/SIGTERM
- Built-in EPP command rate limiting (drop before forwarding to backend)

## Menjalankan
Jalankan dari folder `golang/epp-proxy`:
```bash
cd golang/epp-proxy
go run . \
  -listen :700 \
  -backend 127.0.0.1:1700
```

Atau via environment variable:

```bash
cd golang/epp-proxy && EPP_LISTEN_ADDR=:700 EPP_BACKEND_ADDR=10.10.10.10:7000 go run .
```

## Opsi konfigurasi
- `-listen` / `EPP_LISTEN_ADDR` (default `:700`)
- `-backend` / `EPP_BACKEND_ADDR` (default `127.0.0.1:1700`)
- `-connect-timeout` / `EPP_CONNECT_TIMEOUT` (default `5s`)
- `-idle-timeout` / `EPP_IDLE_TIMEOUT` (default `10m`)
- `-frontend-tls` / `EPP_FRONTEND_TLS` (default `false`)
- `-frontend-cert` / `EPP_FRONTEND_CERT` (default `certs/server.crt`)
- `-frontend-key` / `EPP_FRONTEND_KEY` (default `certs/server.key`)
- `-backend-tls` / `EPP_BACKEND_TLS` (default `false`)
- `-backend-insecure` / `EPP_BACKEND_INSECURE` (default `false`)
- `-rate-limit-max` / `EPP_RATE_LIMIT_MAX` (default `10`)
- `-rate-limit-window` / `EPP_RATE_LIMIT_WINDOW` (default `1m`)
- `-rate-limit-by` / `EPP_RATE_LIMIT_BY` (default `ip_or_username`, opsi: `ip`, `username`, `ip_or_username`)

## Build binary
```bash
go build -o epp-forwarder .
```

## Perilaku rate limit
- Setiap frame perintah EPP dari client dihitung pada window yang dikonfigurasi.
- Jika melebihi limit, proxy **tidak** meneruskan perintah ke backend dan langsung merespon EPP error code `2502` dengan pesan limit exceeded.
- Untuk mode `username`, key diambil dari `<clID>` pada command login; jika belum ada, fallback ke IP client.
