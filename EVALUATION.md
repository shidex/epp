# Evaluasi Singkat Implementasi Java Saat Ini

## Hal yang sudah baik
1. Pipeline Netty sudah memisahkan concern penting: TLS, framing, timeout, logging, dan business handler.
2. Session handling dipisah ke `SessionManager`/`SessionPoolManager` sehingga state koneksi lebih mudah ditelusuri.
3. Ada rate limiting sebelum dan sesudah login yang menurunkan risiko abuse.

## Temuan yang perlu diperhatikan
1. **Port hardcoded** di `EppServer` (`int port = 700`) membuat deployment kurang fleksibel.
2. **Log sangat verbose** (print XML full payload) berisiko mengekspose data sensitif pada produksi.
3. **Hybrid decoder ambigu**: otomatis menerima dua format frame (4-byte prefix dan delimiter `</epp>`), ini memudahkan kompatibilitas tetapi bisa memperluas surface parsing error jika client tidak konsisten.
4. **Tanggung jawab handler cukup besar**: `EppServerHandler` menangani handshake event, login/auth, rate-limit response, backend forwarding, dan logout dalam satu class sehingga sulit diuji unit.
5. **Error handling dominan `System.out/err`** tanpa struktur logging level/field (misal JSON log), menyulitkan observability di skala tinggi.

## Rekomendasi prioritas
1. Externalize `port`, mode TLS, dan backend URL secara konsisten via config/env.
2. Batasi logging payload XML (misalnya sampling + masking data sensitif).
3. Pisahkan logic menjadi komponen lebih kecil (auth flow, command dispatcher, response builder) untuk testability.
4. Tambah integration test framing (valid/invalid length prefix, fragmented frame, mixed-mode frame).

## Status implementasi (update)
- [x] Konfigurasi port server dibuat configurable (`server.port`) dan toggle TLS listener (`server.ssl.enabled`).
- [x] Logging XML dibatasi dengan mode preview (`logging.xml.max.chars`) dan opsi full logging (`logging.xml.full`) plus masking elemen password.
- [ ] Pemecahan handler besar ke beberapa komponen masih belum dilakukan.
- [ ] Integration test framing Java belum ditambahkan.
