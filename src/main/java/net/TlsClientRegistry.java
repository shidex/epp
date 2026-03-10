package net;

import util.ConfigLoader;

import java.util.Arrays;
import java.util.HashSet;
import java.util.Set;

public final class TlsClientRegistry {

    private TlsClientRegistry() {}

    public static boolean isAllowed(String clientId, String fpSha256Hex) {
        if (clientId == null || clientId.isBlank()) return false;
        if (fpSha256Hex == null || fpSha256Hex.isBlank()) return false;

        // key: tls.client.fingerprint.rsp001
        String key = "tls.client.fingerprint." + clientId.trim();
        String raw = ConfigLoader.get(key);

        if (raw == null || raw.isBlank()) {
            // Fail closed: kalau belum ada mapping, jangan izinkan login
            System.err.println("[TLS REGISTRY] No fingerprint mapping for clID=" + clientId + " key=" + key);
            return false;
        }

        Set<String> allowed = parseFingerprints(raw);
        String fp = normalize(fpSha256Hex);

        boolean ok = allowed.contains(fp);
        if (!ok) {
            System.err.println("[TLS REGISTRY] Fingerprint mismatch for clID=" + clientId +
                    " got=" + fp + " allowed=" + allowed.size());
        }
        return ok;
    }

    private static Set<String> parseFingerprints(String raw) {
        // allow comma/space separated
        String[] parts = raw.split("[,\\s]+");
        Set<String> set = new HashSet<>();
        Arrays.stream(parts)
                .map(String::trim)
                .filter(s -> !s.isBlank())
                .map(TlsClientRegistry::normalize)
                .forEach(set::add);
        return set;
    }

    private static String normalize(String fp) {
        // normalize to lowercase hex without colons
        return fp.trim().toLowerCase().replace(":", "");
    }
}
