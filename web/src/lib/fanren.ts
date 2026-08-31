export const FANREN_DEFAULT_BASE_URL = "https://fanrenapi.com";

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
