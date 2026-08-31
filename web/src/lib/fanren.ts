export const FANREN_DEFAULT_BASE_URL = "https://fanrenapi.com";
export const FANREN_SSO_ENABLED = process.env.NEXT_PUBLIC_FANREN_SSO === "1" || process.env.NEXT_PUBLIC_FANREN_SSO === "true";
export const FANREN_TOKEN_ID_HEADER = "X-Fanren-Token-ID";

const FANREN_HOSTS = new Set(["fanrenapi.com", "www.fanrenapi.com", "cdn.fanrenapi.com", "console.fanrenapi.eu.cc"]);

export function isFanrenBaseUrl(baseUrl: string) {
    try {
        const host = new URL(baseUrl.trim()).hostname.toLowerCase();
        return FANREN_HOSTS.has(host) || host.endsWith(".fanrenapi.com");
    } catch {
        return false;
    }
}

export function supportsFanrenImageJobs(model: string) {
    return model.trim().toLowerCase().startsWith("gpt-image");
}

export function isFanrenIntegratedConfig(config: { channelMode: string; baseUrl?: string; publicChannels?: Array<{ baseUrl?: string }> }) {
    return FANREN_SSO_ENABLED && config.channelMode === "remote";
}
