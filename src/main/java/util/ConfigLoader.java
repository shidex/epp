package util;

import java.io.FileInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.Properties;

public class ConfigLoader {
    private static final Properties props = new Properties();
    private static final Object LOCK = new Object();

    /**
     * Bisa diisi lewat JVM arg:
     * -Dconfig.file=/opt/epp/config.properties
     *
     * Kalau tidak diisi, default ke ./config.properties
     */
    private static final String CONFIG_PATH =
            System.getProperty("config.file", "./config.properties");

    private static volatile long lastModified = -1L;

    static {
        loadProperties();
    }

    public static String get(String key) {
        reloadIfNeeded();
        return props.getProperty(key);
    }

    public static String get(String key, String defaultValue) {
        reloadIfNeeded();
        return props.getProperty(key, defaultValue);
    }

    public static void forceReload() {
        loadProperties();
    }

    private static void reloadIfNeeded() {
        try {
            Path path = Paths.get(CONFIG_PATH);

            if (!Files.exists(path)) {
                return;
            }

            long currentLastModified = Files.getLastModifiedTime(path).toMillis();
            if (currentLastModified != lastModified) {
                synchronized (LOCK) {
                    currentLastModified = Files.getLastModifiedTime(path).toMillis();
                    if (currentLastModified != lastModified) {
                        loadProperties();
                    }
                }
            }
        } catch (Exception e) {
            System.err.println("[CONFIG] Failed to reload config: " + e.getMessage());
        }
    }

    private static void loadProperties() {
        synchronized (LOCK) {
            Properties temp = new Properties();

            try {
                Path path = Paths.get(CONFIG_PATH);

                if (Files.exists(path)) {
                    try (InputStream input = new FileInputStream(path.toFile())) {
                        temp.load(input);
                    }
                    lastModified = Files.getLastModifiedTime(path).toMillis();
                    System.out.println("[CONFIG] Loaded external config: " + path.toAbsolutePath());
                } else {
                    try (InputStream input = ConfigLoader.class.getClassLoader()
                            .getResourceAsStream("config.properties")) {

                        if (input == null) {
                            throw new IOException("config.properties not found in external path or classpath");
                        }

                        temp.load(input);
                        lastModified = -1L;
                        System.out.println("[CONFIG] Loaded classpath config.properties");
                    }
                }

                props.clear();
                props.putAll(temp);

            } catch (IOException e) {
                throw new RuntimeException("Failed to load config.properties", e);
            }
        }
    }
}