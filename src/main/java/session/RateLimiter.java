package session;

import io.netty.channel.Channel;
import io.netty.channel.ChannelId;
import util.ConfigLoader;

import java.io.BufferedWriter;
import java.io.FileWriter;
import java.io.IOException;
import java.io.PrintWriter;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Date;
import java.util.Deque;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

public class RateLimiter {

    private static final Map<String, Deque<Long>> ipRequestMap = new ConcurrentHashMap<>();
    private static final Map<String, Deque<Long>> clientRequestMap = new ConcurrentHashMap<>();
    private static final Map<ChannelId, Deque<Long>> channelRequestMap = new ConcurrentHashMap<>();

    private static final List<RateLimitRule> IP_RULES;
    private static final List<RateLimitRule> CLIENT_RULES;
    private static final List<RateLimitRule> CHANNEL_RULES;

    static {
        IP_RULES = loadRules("ratelimit.ip.rules", "20/second,100/minute");
        CLIENT_RULES = loadRules("ratelimit.client.rules", "50/second,500/minute");
        CHANNEL_RULES = loadRules("ratelimit.channel.rules", "10/second,60/minute");

        System.out.println("[RATE LIMIT] Loaded IP rules      : " + IP_RULES);
        System.out.println("[RATE LIMIT] Loaded CLIENT rules  : " + CLIENT_RULES);
        System.out.println("[RATE LIMIT] Loaded CHANNEL rules : " + CHANNEL_RULES);
    }

    private RateLimiter() {
    }

    public static boolean isIpRateLimited(String ip) {
        if (ip == null || ip.trim().isEmpty()) {
            return false;
        }
        return isRateLimited(ipRequestMap, ip, IP_RULES, "IP");
    }

    public static boolean isClientRateLimited(String clientId) {
        if (clientId == null || clientId.trim().isEmpty()) {
            return false;
        }
        return isRateLimited(clientRequestMap, clientId, CLIENT_RULES, "CLIENT");
    }

    public static boolean isChannelRateLimited(Channel channel) {
        if (channel == null) {
            return false;
        }
        return isRateLimited(channelRequestMap, channel.id(), CHANNEL_RULES, "CHANNEL");
    }

    public static boolean isPreLoginRateLimited(String ip, Channel channel) {
        if (isIpRateLimited(ip)) {
            return true;
        }

        if (isChannelRateLimited(channel)) {
            return true;
        }

        return false;
    }

    public static boolean isRateLimited(String ip, String clientId, Channel channel) {
        if (isIpRateLimited(ip)) {
            return true;
        }

        if (isChannelRateLimited(channel)) {
            return true;
        }

        if (clientId != null && !clientId.trim().isEmpty() && isClientRateLimited(clientId)) {
            return true;
        }

        return false;
    }

    public static void removeChannel(Channel channel) {
        if (channel != null) {
            channelRequestMap.remove(channel.id());
        }
    }

    public static void removeIp(String ip) {
        if (ip != null) {
            ipRequestMap.remove(ip);
        }
    }

    public static void removeClient(String clientId) {
        if (clientId != null) {
            clientRequestMap.remove(clientId);
        }
    }

    private static <K> boolean isRateLimited(Map<K, Deque<Long>> requestMap, K key, List<RateLimitRule> rules, String scope) {
        long now = System.currentTimeMillis();

        Deque<Long> timestamps = requestMap.computeIfAbsent(key, k -> new ArrayDeque<>());

        synchronized (timestamps) {
            long maxWindowMs = getMaxWindow(rules);

            while (!timestamps.isEmpty() && (now - timestamps.peekFirst()) > maxWindowMs) {
                timestamps.pollFirst();
            }

            for (RateLimitRule rule : rules) {
                int countInWindow = countRequestsWithinWindow(timestamps, now, rule.getWindowMs());

                if (countInWindow >= rule.getMaxRequests()) {
                    logRateLimited(scope, String.valueOf(key), countInWindow, rule);
                    return true;
                }
            }

            timestamps.addLast(now);
            return false;
        }
    }

    private static int countRequestsWithinWindow(Deque<Long> timestamps, long now, long windowMs) {
        int count = 0;
        for (Long ts : timestamps) {
            if ((now - ts) <= windowMs) {
                count++;
            }
        }
        return count;
    }

    private static long getMaxWindow(List<RateLimitRule> rules) {
        long max = 60_000L;
        for (RateLimitRule rule : rules) {
            if (rule.getWindowMs() > max) {
                max = rule.getWindowMs();
            }
        }
        return max;
    }

    private static List<RateLimitRule> loadRules(String configKey, String defaultValue) {
        List<RateLimitRule> rules = new ArrayList<>();

        String rawConfig;
        try {
            rawConfig = ConfigLoader.get(configKey);
        } catch (Exception e) {
            rawConfig = null;
        }

        if (rawConfig == null || rawConfig.trim().isEmpty()) {
            System.err.println("[WARN] " + configKey + " not found, using default: " + defaultValue);
            rawConfig = defaultValue;
        }

        String[] parts = rawConfig.split(",");
        for (String part : parts) {
            String item = part.trim().toLowerCase();
            if (item.isEmpty()) {
                continue;
            }

            try {
                rules.add(parseRule(item));
            } catch (Exception e) {
                System.err.println("[WARN] Failed to parse " + configKey + ": " + item + " | " + e.getMessage());
            }
        }

        if (rules.isEmpty()) {
            System.err.println("[WARN] No valid rules found for " + configKey + ", using default: " + defaultValue);
            String[] fallbackParts = defaultValue.split(",");
            for (String fallback : fallbackParts) {
                rules.add(parseRule(fallback.trim().toLowerCase()));
            }
        }

        return rules;
    }

    /**
     * Supported formats:
     * 10/second
     * 10/sec
     * 60/minute
     * 60/min
     * 1000/hour
     * 1000/hr
     */
    private static RateLimitRule parseRule(String value) {
        String[] arr = value.split("/");
        if (arr.length != 2) {
            throw new IllegalArgumentException("Format must be <count>/<unit>");
        }

        int maxRequests = Integer.parseInt(arr[0].trim());
        String unit = arr[1].trim();

        if (maxRequests <= 0) {
            throw new IllegalArgumentException("maxRequests must be > 0");
        }

        long windowMs;
        String normalizedUnit;

        switch (unit) {
            case "sec":
            case "second":
            case "seconds":
                windowMs = 1_000L;
                normalizedUnit = "second";
                break;

            case "min":
            case "minute":
            case "minutes":
                windowMs = 60_000L;
                normalizedUnit = "minute";
                break;

            case "hr":
            case "hour":
            case "hours":
                windowMs = 3_600_000L;
                normalizedUnit = "hour";
                break;

            default:
                throw new IllegalArgumentException("Unsupported unit: " + unit);
        }

        return new RateLimitRule(maxRequests, windowMs, normalizedUnit);
    }

    private static void logRateLimited(String scope, String key, int count, RateLimitRule rule) {
        String log = String.format(
                "[RATE LIMIT] %s %s exceeded %d req/%s (current=%d) @ %s",
                scope,
                key,
                rule.getMaxRequests(),
                rule.getUnitLabel(),
                count,
                new Date()
        );

        System.err.println(log);

        try (FileWriter fw = new FileWriter("ratelimit.log", true);
             BufferedWriter bw = new BufferedWriter(fw);
             PrintWriter out = new PrintWriter(bw)) {
            out.println(log);
        } catch (IOException e) {
            System.err.println("[LOGGER] Failed to write rate limit log: " + e.getMessage());
        }
    }
}