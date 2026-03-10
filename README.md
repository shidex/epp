# EPP Server (Java)

## Overview
This application is an **EPP Server written in Java** using **Netty** for TCP/TLS communication.  
It implements the **Extensible Provisioning Protocol (EPP)** used by domain registries and registrars.

Main features:

- TLS connection support
- Automatic EPP greeting
- Registrar login handling
- Session management
- Rate limiting per IP, ClientId, Channel
- Backend API integration for authorization and command processing

---

# Requirements

Before running the server, ensure the system has:

- **Java 17**
- **Maven 3.8+**
- **OpenSSL**
- TLS certificate files

---

# Install Java

Check if Java is installed:

```bash
java -version
```

If not installed (Ubuntu/Debian):

```bash
sudo apt update
sudo apt install openjdk-17-jdk -y
```

Verify installation:

```bash
java -version
```

---

# Install Maven

Check Maven:

```bash
mvn -version
```

If Maven is not installed:

```bash
sudo apt install maven -y
```

---

# Project Structure

Example project structure:

```
epp-server/
│
├── certs/
│   ├── server.crt
│   ├── server.key
│   └── cacert.pem
│
├── src/
│
├── config.properties
│
└── pom.xml
```

---

# TLS Certificates

The server requires the following files:

```
server.crt
server.key
cacert.pem
```

Place them inside the `certs/` directory.

Example:

```bash
mkdir certs

cp /path/server.crt certs/
cp /path/server.key certs/
cp /path/cacert.pem certs/
```

---

# Configuration

The application uses `config.properties` for runtime configuration.

Example configuration:

```properties
server.port=700
server.ssl.enabled=true
authbackend.url=http://be.pandi.id:8080/PANDI-REGISTRAR-0.1/authRegistrar/
backend.url=http://be.pandi.id:8080/PANDI-CORE-0.1/processepp/
logoutbackend.url=http://be.pandi.id:8080/PANDI-REGISTRAR-0.1/logoutRegistrar/
enable.validation=false
idle.timeout.seconds=600
tls.client.auth=REQUIRE
logging.xml.full=false
logging.xml.max.chars=512
ratelimit.ip.rules=10/second,60/minute
ratelimit.client.rules=50/second,500/minute
ratelimit.channel.rules=10/second,60/minute
ratelimit.read.ip.rules=30/second,200/minute
ratelimit.write.ip.rules=10/second,60/minute
ratelimit.read.client.rules=100/second,800/minute
ratelimit.write.client.rules=50/second,500/minute
```

## Configuration Description

| Parameter | Description |
|-----------|-------------|
| `server.port` | Port listener EPP server (default `700`) |
| `server.ssl.enabled` | Aktif/nonaktif TLS pada listener server |
| `authbackend.url` | Registrar authentication API endpoint |
| `backend.url` | Backend EPP command processing API endpoint |
| `logoutbackend.url` | Registrar logout API endpoint |
| `enable.validation` | Enable or disable XML/XSD validation |
| `idle.timeout.seconds` | Idle session timeout in seconds |
| `tls.client.auth` | TLS client certificate mode: `NONE`, `OPTIONAL`, or `REQUIRE` |
| `logging.xml.full` | Jika `true`, log XML penuh (sudah masking pw/newPW) |
| `logging.xml.max.chars` | Batas preview XML saat `logging.xml.full=false` |
| `ratelimit.ip.rules` | Rate limit rules per client IP |
| `ratelimit.client.rules` | Rate limit rules per authenticated client / registrar |
| `ratelimit.channel.rules` | Baseline rate limit rules per connection / channel |
| `ratelimit.read.ip.rules` | Additional read-command rate limit rules per client IP |
| `ratelimit.write.ip.rules` | Additional write-command rate limit rules per client IP |
| `ratelimit.read.client.rules` | Additional read-command rate limit rules per authenticated client |
| `ratelimit.write.client.rules` | Additional write-command rate limit rules per authenticated client |

## Notes

- `tls.client.auth=REQUIRE` means mutual TLS client certificate is mandatory.
- `enable.validation=false` disables XML/XSD validation at runtime.
- Rate limit rules use a multi-window format such as `10/second,60/minute`.
- Read commands are treated as `check`, `info`, and `poll`; other commands are treated as write commands.
- If `ratelimit.read.*` / `ratelimit.write.*` are omitted, the server falls back to existing baseline `ratelimit.ip.rules` / `ratelimit.client.rules`.

# Build Application

Navigate to the project directory:

```bash
cd epp-server
```

Build using Maven:

```bash
mvn clean package -DskipTests
```

After successful build, the JAR file will appear in:

```
target/epp-server.jar
```

---

# Run the Server

Run the server using:

```bash
java -jar target/epp-server.jar
```

Optional memory configuration:

```bash
java -Xms256m -Xmx1024m -jar target/epp-server.jar
```

If the `config.properties` file is located outside the project directory, you can specify it using a JVM system property:

```bash
java -Dconfig.file=/opt/epp/config.properties -jar target/epp-server.jar
```

---

# Run in Background

Example:

```bash
nohup java -jar target/epp-server.jar > epp.log 2>&1 &
```

Check running process:

```bash
ps aux | grep epp-server
```

View logs:

```bash
tail -f epp.log
```

---

# Stop the Server

Find the process:

```bash
ps aux | grep epp-server
```

Terminate it:

```bash
kill -9 PID
```

---

# Testing the Server

## Test Port

```bash
nc -vz 127.0.0.1 700
```

## Test TLS Connection

```bash
openssl s_client -connect 127.0.0.1:700
```

If TLS handshake succeeds, the server should send an EPP:

```
<greeting>
```

---

# TLS Client Authentication Modes

The server supports three TLS client authentication modes.

### REQUIRE

Client must present a valid certificate during TLS handshake.

```
tls.client.auth=REQUIRE
```

### OPTIONAL

TLS handshake succeeds even without a client certificate, but certificate validation must be handled during login.

```
tls.client.auth=OPTIONAL
```

### NONE

Server does not request client certificates.

```
tls.client.auth=NONE
```

---

# Troubleshooting

## TLS Handshake Errors

Verify:

- `server.crt`
- `server.key`
- `cacert.pem`

Ensure the certificate chain is valid.

---

## Port Not Listening

Check:

```bash
ss -tulpn | grep 700
```

---

## Maven Build Errors

Retry build:

```bash
mvn clean package
```

---

# Security Notes

For production deployment:

- Use valid TLS certificates
- Prefer `ClientAuth.REQUIRE`
- Restrict access to the EPP port
- Avoid logging registrar passwords
- Enable rate limiting

---

# Author

EPP Server Development Team
