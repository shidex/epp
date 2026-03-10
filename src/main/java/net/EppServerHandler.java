package net;

import dto.RegistrarDto;
import io.netty.channel.ChannelFutureListener;
import io.netty.channel.ChannelHandlerContext;
import io.netty.channel.ChannelInboundHandlerAdapter;
import io.netty.handler.ssl.SslHandler;
import io.netty.handler.ssl.SslHandshakeCompletionEvent;
import model.EppRequest;
import session.SessionContext;
import session.SessionManager;
import session.SessionPoolManager;
import session.SessionState;
import util.ConfigLoader;
import util.EPPBackendApiUtil;
import util.XmlUtils;

import javax.net.ssl.SSLPeerUnverifiedException;
import javax.net.ssl.SSLSession;
import javax.xml.bind.UnmarshalException;
import java.net.InetSocketAddress;
import java.security.MessageDigest;
import java.security.cert.X509Certificate;
import java.util.UUID;
import java.util.regex.Pattern;

public class EppServerHandler extends ChannelInboundHandlerAdapter {

    private static final Pattern PW_PATTERN = Pattern.compile("(<pw>)(.*?)(</pw>)", Pattern.CASE_INSENSITIVE | Pattern.DOTALL);
    private static final Pattern NEW_PW_PATTERN = Pattern.compile("(<newPW>)(.*?)(</newPW>)", Pattern.CASE_INSENSITIVE | Pattern.DOTALL);

    private void logIncomingXml(ChannelHandlerContext ctx, String message) {
        boolean fullLogging = Boolean.parseBoolean(ConfigLoader.get("logging.xml.full", "false"));
        int maxChars = Integer.parseInt(ConfigLoader.get("logging.xml.max.chars", "512"));

        String sanitized = sanitizeSensitiveXml(message);
        if (fullLogging) {
            System.out.println("[RECEIVED XML] From " + ctx.channel().remoteAddress() + ":\n" + sanitized);
            return;
        }

        String preview = sanitized.length() <= maxChars
                ? sanitized
                : sanitized.substring(0, maxChars) + "...(truncated)";

        System.out.println("[RECEIVED XML] From " + ctx.channel().remoteAddress()
                + " size=" + message.length()
                + " preview=\"" + preview + "\"");
    }

    private String sanitizeSensitiveXml(String xml) {
        String masked = PW_PATTERN.matcher(xml).replaceAll("$1***$3");
        return NEW_PW_PATTERN.matcher(masked).replaceAll("$1***$3");
    }


    @Override
    public void userEventTriggered(ChannelHandlerContext ctx, Object evt) {
        if (evt instanceof SslHandshakeCompletionEvent handshakeEvent) {
            if (handshakeEvent.isSuccess()) {
                System.out.println("[TLS] Handshake sukses: " + ctx.channel().remoteAddress());

                // Optional debug untuk melihat apakah client membawa certificate
                logClientCertPresence(ctx);

                // Setelah handshake sukses, kirim greeting
                SessionManager.setSessionState(ctx.channel(), SessionState.SENT_GREETING);

                String response = XmlUtils.buildGreetingResponse();
                ctx.writeAndFlush(response).addListener(future -> {
                    if (future.isSuccess()) {
                        System.out.println("[DEBUG] Greeting flushed successfully to " + ctx.channel().remoteAddress());
                    } else {
                        System.err.println("[ERROR] Failed to flush greeting: " + safeMsg(future.cause()));
                        if (future.cause() != null) {
                            future.cause().printStackTrace();
                        }
                    }
                });
            } else {
                System.err.println("[TLS] TLS handshake gagal: " + safeMsg(handshakeEvent.cause()));
                ctx.close();
            }
        } else {
            ctx.fireUserEventTriggered(evt);
        }
    }

    @Override
    public void channelRead(ChannelHandlerContext ctx, Object msg) {
        if (!(msg instanceof String message)) {
            System.err.println("[ERROR] Unknown message type: " + msg.getClass());
            ctx.close();
            return;
        }

        logIncomingXml(ctx, message);

        String sessionId = UUID.randomUUID().toString();
        String clientIp = getClientIp(ctx);
        SessionState state = SessionManager.getSessionState(ctx.channel());

        try {
            EppRequest request = XmlUtils.parseXml(message);

            switch (state) {
                case SENT_GREETING -> {
                    if (SessionManager.isPreLoginRateLimited(clientIp, ctx.channel())) {
                        System.err.println("[RATE LIMIT] Pre-login limit exceeded. ip=" + clientIp
                                + " channel=" + ctx.channel().id().asShortText());

                        ctx.writeAndFlush(XmlUtils.buildRateLimitResponse(sessionId))
                                .addListener(future -> {
                                    if (future.isSuccess()) {
                                        System.out.println("[DEBUG] Rate-limit response flushed to " + ctx.channel().remoteAddress());
                                    } else {
                                        System.err.println("[ERROR] Failed to flush rate-limit response: " + safeMsg(future.cause()));
                                        if (future.cause() != null) {
                                            future.cause().printStackTrace();
                                        }
                                    }
                                    ctx.close();
                                });
                        return;
                    }

                    handleLogin(ctx, request, sessionId, clientIp);
                }

                case AUTHENTICATED -> {
                    SessionContext session = SessionManager.getSessionByChannel(ctx.channel());
                    String clientId = session != null ? session.getClientId() : null;
                    boolean writeCommand = isWriteCommand(message);

                    if (SessionManager.isRateLimited(clientIp, clientId, ctx.channel(), writeCommand)) {
                        System.err.println("[RATE LIMIT] Post-login limit exceeded. type=" + (writeCommand ? "WRITE" : "READ")
                                + " ip=" + clientIp
                                + " clID=" + clientId
                                + " channel=" + ctx.channel().id().asShortText());

                        ctx.writeAndFlush(XmlUtils.buildRateLimitResponse(sessionId))
                                .addListener(future -> {
                                    if (future.isSuccess()) {
                                        System.out.println("[DEBUG] Rate-limit response flushed to " + ctx.channel().remoteAddress());
                                    } else {
                                        System.err.println("[ERROR] Failed to flush rate-limit response: " + safeMsg(future.cause()));
                                        if (future.cause() != null) {
                                            future.cause().printStackTrace();
                                        }
                                    }
                                    ctx.close();
                                });
                        return;
                    }

                    handleEppCommand(ctx, request, message);
                }

                default -> {
                    System.err.println("[SESSION] Invalid state " + state + " from " + ctx.channel().remoteAddress());
                    ctx.writeAndFlush(XmlUtils.buildErrorResponse("Invalid session state"))
                            .addListener(f -> ctx.close());
                }
            }

        } catch (Exception e) {
            System.err.println("[ERROR] Failed to process request from " + ctx.channel().remoteAddress()
                    + ": " + safeMsg(e));

            if (e.getCause() instanceof UnmarshalException) {
                ctx.writeAndFlush(XmlUtils.buildPolicyErrorResponse("Malformed XML"))
                        .addListener(f -> ctx.close());
            } else {
                ctx.writeAndFlush(XmlUtils.buildErrorResponse("Unexpected server error"))
                        .addListener(f -> ctx.close());
            }
        }
    }


    private boolean isWriteCommand(String rawMessage) {
        if (rawMessage == null) {
            return true;
        }

        String xml = rawMessage.toLowerCase();

        if (xml.contains("<check") || xml.contains(":check")
                || xml.contains("<info") || xml.contains(":info")
                || xml.contains("<poll") || xml.contains(":poll")) {
            return false;
        }

        return true;
    }

    private void handleLogin(ChannelHandlerContext ctx, EppRequest request, String sessionId, String clientIp) {
        if (request.command == null || request.command.login == null) {
            ctx.writeAndFlush(XmlUtils.buildErrorResponse("Expected <login>"))
                    .addListener(f -> ctx.close());
            return;
        }

        String clientId = request.command.login.clientId;
        String password = request.command.login.password;
        String newPassword = request.command.login.newPassword;
        String clTRID = request.command.clTRID;

        // Wajib ada client certificate
        X509Certificate leafCert = getClientLeafCert(ctx);
        if (leafCert == null) {
            System.err.println("[AUTH FAIL] No client certificate presented. ip=" + clientIp + " clID=" + clientId);
            ctx.writeAndFlush(XmlUtils.buildAuthFailResponse(sessionId))
                    .addListener(f -> ctx.close());
            return;
        }

        // Hitung fingerprint certificate
        String certFp;
        try {
            certFp = sha256Fingerprint(leafCert);
        } catch (Exception e) {
            System.err.println("[AUTH FAIL] Failed to compute cert fingerprint. ip=" + clientIp
                    + " clID=" + clientId + " err=" + safeMsg(e));
            ctx.writeAndFlush(XmlUtils.buildAuthFailResponse(sessionId))
                    .addListener(f -> ctx.close());
            return;
        }

        System.out.println("[CERT] clID=" + clientId + " fp_sha256=" + certFp);

        RegistrarDto dto = EPPBackendApiUtil.processAuthorization(
                ConfigLoader.get("authbackend.url"),
                clientId,
                password,
                newPassword,
                certFp,
                clientIp,
                ""
        );

        if (dto != null && "00".equalsIgnoreCase(dto.getResponseCode())) {
            SessionContext context = SessionManager.createSession(sessionId, clientId, ctx.channel());
            SessionManager.setSessionState(ctx.channel(), SessionState.AUTHENTICATED);

            // Simpan token dari backend
            SessionManager.setActiveToken(ctx.channel(), dto.getEppSessionToken());

            // Simpan fingerprint cert
            SessionManager.setCertificateHash(ctx.channel(), certFp);

            SessionPoolManager.register(clientId, context);

            System.out.println("[AUTH SUCCESS] ip=" + clientIp + " clID=" + clientId
                    + " channel=" + ctx.channel().id().asShortText());

            ctx.writeAndFlush(XmlUtils.buildLoginResponse(sessionId, clTRID));
        } else {
            System.err.println("[AUTH FAIL] Backend auth rejected. ip=" + clientIp + " clID=" + clientId);
            ctx.writeAndFlush(XmlUtils.buildAuthFailResponse(sessionId))
                    .addListener(f -> ctx.close());
        }
    }

    private void handleEppCommand(ChannelHandlerContext ctx, EppRequest request, String rawMessage) {
        if (rawMessage.contains("<logout")) {
            String clTRID = (request.command != null) ? request.command.clTRID : "";
            String svTRID = UUID.randomUUID().toString();

            ctx.writeAndFlush(XmlUtils.buildLogoutResponse(svTRID, clTRID))
                    .addListener(f -> ctx.close());

            SessionManager.setSessionState(ctx.channel(), SessionState.END_SESSION);
            return;
        }

        String token = SessionManager.getActiveToken(ctx.channel());
        String backendUrl = ConfigLoader.get("backend.url");

        if (token == null || token.isBlank()) {
            System.err.println("[AUTH] Missing backend session token for channel="
                    + ctx.channel().id().asShortText());

            ctx.writeAndFlush(XmlUtils.buildErrorResponse("Missing session token"))
                    .addListener(f -> ctx.close());
            return;
        }

        String response = EPPBackendApiUtil.processEPPCommand(backendUrl, rawMessage, token);
        ctx.writeAndFlush(response);
    }

    private void logClientCertPresence(ChannelHandlerContext ctx) {
        SslHandler ssl = ctx.pipeline().get(SslHandler.class);
        if (ssl == null) {
            System.out.println("[TLS] No SslHandler in pipeline");
            return;
        }

        SSLSession s = ssl.engine().getSession();
        try {
            var peer = s.getPeerCertificates();
            System.out.println("[TLS] Client certificate presented, chainLen=" + (peer == null ? 0 : peer.length));
        } catch (SSLPeerUnverifiedException e) {
            System.out.println("[TLS] No client certificate presented (OPTIONAL mode or client omitted cert)");
        } catch (Exception e) {
            System.out.println("[TLS] Could not inspect peer certs: " + safeMsg(e));
        }
    }

    private X509Certificate getClientLeafCert(ChannelHandlerContext ctx) {
        SslHandler sslHandler = ctx.pipeline().get(SslHandler.class);
        if (sslHandler == null) {
            return null;
        }

        SSLSession session = sslHandler.engine().getSession();
        try {
            var peer = session.getPeerCertificates();
            if (peer == null || peer.length == 0) {
                return null;
            }

            if (!(peer[0] instanceof X509Certificate)) {
                return null;
            }

            return (X509Certificate) peer[0];
        } catch (SSLPeerUnverifiedException e) {
            return null;
        }
    }

    private String sha256Fingerprint(X509Certificate cert) throws Exception {
        byte[] der = cert.getEncoded();
        byte[] digest = MessageDigest.getInstance("SHA-256").digest(der);

        StringBuilder sb = new StringBuilder();
        for (byte b : digest) {
            sb.append(String.format("%02x", b));
        }
        return sb.toString();
    }

    private String getClientIp(ChannelHandlerContext ctx) {
        if (!(ctx.channel().remoteAddress() instanceof InetSocketAddress socket)) {
            return null;
        }

        if (socket.getAddress() == null) {
            return null;
        }

        return socket.getAddress().getHostAddress();
    }

    private String safeMsg(Throwable t) {
        if (t == null) {
            return "null";
        }
        return (t.getMessage() != null) ? t.getMessage() : t.getClass().getName();
    }

    @Override
    public void channelInactive(ChannelHandlerContext ctx) {
        System.out.println("[EPP] Disconnected: " + ctx.channel().remoteAddress());
        SessionManager.removeSessionByChannel(ctx.channel());
        SessionPoolManager.unregister(ctx.channel());
    }

    @Override
    public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) {
        System.err.println("[ERROR] " + safeMsg(cause));
        cause.printStackTrace();
        ctx.close();
    }
}