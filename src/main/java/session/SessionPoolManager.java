package session;

import io.netty.channel.Channel;

import java.util.*;
import java.util.concurrent.ConcurrentHashMap;

public class SessionPoolManager {
    private static final Map<String, List<SessionContext>> pool = new ConcurrentHashMap<>();

    public static void register(String clientId, SessionContext context) {
        pool.computeIfAbsent(clientId, k -> Collections.synchronizedList(new ArrayList<>())).add(context);
        System.out.println("[SESSION POOL] Registered: " + context.getSessionId() + " for client: " + clientId);
    }

    public static void unregister(Channel channel) {
        for (Map.Entry<String, List<SessionContext>> entry : pool.entrySet()) {
            entry.getValue().removeIf(ctx -> ctx.getChannel().equals(channel));
        }
        System.out.println("[SESSION POOL] Removed channel from pool: " + channel.id().asShortText());
    }

    public static List<SessionContext> getSessions(String clientId) {
        return pool.getOrDefault(clientId, Collections.emptyList());
    }

    public static SessionContext findByChannel(Channel channel) {
        return pool.values().stream()
            .flatMap(List::stream)
            .filter(ctx -> ctx.getChannel().equals(channel))
            .findFirst()
            .orElse(null);
    }

    public static void debugPool() {
        System.out.println("[SESSION POOL] Current pool state:");
        pool.forEach((clientId, list) -> {
            System.out.println("  Client " + clientId + " has " + list.size() + " session(s).");
        });
    }
}
