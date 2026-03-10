package session;

import io.netty.channel.Channel;
import io.netty.channel.ChannelId;

import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.stream.Collectors;

public class SessionManager {
    private static final Map<String, SessionContext> sessions = new ConcurrentHashMap<>();
    private static final Map<ChannelId, SessionContext> sessionsByChannel = new ConcurrentHashMap<>();
    private static final Map<ChannelId, SessionState> sessionStates = new ConcurrentHashMap<>();

    private SessionManager() {
    }

    public static void debugSessionMap() {
        System.out.println("[DEBUG] SessionStates content:");
        sessionStates.forEach((key, val) ->
                System.out.println("  - " + key.asShortText() + " -> " + val));
    }

    public static SessionContext createSession(String sessionId, String clientId, Channel channel) {
        SessionContext ctx = new SessionContext(sessionId, clientId, channel);
        sessions.put(sessionId, ctx);
        sessionsByChannel.put(channel.id(), ctx);
        return ctx;
    }

    public static void removeSessionByChannel(Channel channel) {
        if (channel == null) {
            return;
        }

        ChannelId channelId = channel.id();
        SessionContext removed = sessionsByChannel.remove(channelId);

        if (removed != null) {
            sessions.remove(removed.getSessionId());
        } else {
            sessions.entrySet().removeIf(entry -> entry.getValue().getChannel().equals(channel));
        }

        sessionStates.remove(channelId);
        RateLimiter.removeChannel(channel);
    }

    public static SessionContext getSession(String sessionId) {
        return sessionId != null ? sessions.get(sessionId) : null;
    }

    public static SessionContext getSessionByChannel(Channel channel) {
        return channel != null ? sessionsByChannel.get(channel.id()) : null;
    }

    public static void setSessionState(Channel channel, SessionState state) {
        if (channel == null || state == null) {
            return;
        }

        sessionStates.put(channel.id(), state);
    }

    public static SessionState getSessionState(Channel channel) {
        if (channel == null) {
            return SessionState.WAITING_FOR_CLIENT;
        }

        return sessionStates.getOrDefault(channel.id(), SessionState.WAITING_FOR_CLIENT);
    }

    public static String getActiveToken(Channel channel) {
        SessionContext ctx = getSessionByChannel(channel);
        return ctx != null ? ctx.getActiveToken() : null;
    }

    public static void setActiveToken(Channel channel, String token) {
        SessionContext ctx = getSessionByChannel(channel);
        if (ctx != null) {
            ctx.setActiveToken(token);
        }
    }

    public static String getCertificateHash(Channel channel) {
        SessionContext ctx = getSessionByChannel(channel);
        return ctx != null ? ctx.getCertificateHash() : null;
    }

    public static void setCertificateHash(Channel channel, String certificateHash) {
        SessionContext ctx = getSessionByChannel(channel);
        if (ctx != null) {
            ctx.setCertificateHash(certificateHash);
        }
    }

    public static String getClientId(Channel channel) {
        SessionContext ctx = getSessionByChannel(channel);
        return ctx != null ? ctx.getClientId() : null;
    }

    public static String debugSessionKeys() {
        return sessionStates.keySet().stream()
                .map(ChannelId::asShortText)
                .collect(Collectors.joining(", "));
    }

    public static boolean isPreLoginRateLimited(String ip, Channel channel) {
        return RateLimiter.isPreLoginRateLimited(ip, channel);
    }

    public static boolean isPostLoginRateLimited(String ip, Channel channel) {
        SessionContext ctx = getSessionByChannel(channel);
        String clientId = ctx != null ? ctx.getClientId() : null;
        return RateLimiter.isRateLimited(ip, clientId, channel, true);
    }

    public static boolean isRateLimited(String ip, String clientId, Channel channel) {
        return RateLimiter.isRateLimited(ip, clientId, channel);
    }

    public static boolean isRateLimited(String ip, String clientId, Channel channel, boolean writeCommand) {
        return RateLimiter.isRateLimited(ip, clientId, channel, writeCommand);
    }
}
