package session;

import io.netty.channel.Channel;

public class SessionContext {
    private final String sessionId;
    private final String clientId;
    private final Channel channel;

    private String activeToken;
    private String certificateHash;

    public SessionContext(String sessionId, String clientId, Channel channel) {
        this.sessionId = sessionId;
        this.clientId = clientId;
        this.channel = channel;
    }

    public String getSessionId() {
        return sessionId;
    }

    public String getClientId() {
        return clientId;
    }

    public Channel getChannel() {
        return channel;
    }

    public String getActiveToken() {
        return activeToken;
    }

    public void setActiveToken(String activeToken) {
        this.activeToken = activeToken;
    }

    public String getCertificateHash() {
        return certificateHash;
    }

    public void setCertificateHash(String certificateHash) {
        this.certificateHash = certificateHash;
    }

    @Override
    public String toString() {
        return "SessionContext{" +
                "sessionId='" + sessionId + '\'' +
                ", clientId='" + clientId + '\'' +
                ", channel=" + (channel != null ? channel.id().asShortText() : "null") +
                '}';
    }
}