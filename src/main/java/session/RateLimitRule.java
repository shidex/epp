package session;

public class RateLimitRule {
    private final int maxRequests;
    private final long windowMs;
    private final String unitLabel;

    public RateLimitRule(int maxRequests, long windowMs, String unitLabel) {
        this.maxRequests = maxRequests;
        this.windowMs = windowMs;
        this.unitLabel = unitLabel;
    }

    public int getMaxRequests() {
        return maxRequests;
    }

    public long getWindowMs() {
        return windowMs;
    }

    public String getUnitLabel() {
        return unitLabel;
    }

    @Override
    public String toString() {
        return maxRequests + "/" + unitLabel;
    }
}