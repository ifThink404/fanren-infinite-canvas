const configuredBasePath = (process.env.NEXT_PUBLIC_BASE_PATH || "").trim();
const normalizedBasePath = configuredBasePath.replace(/^\/+|\/+$/g, "");

export const APP_BASE_PATH = normalizedBasePath ? `/${normalizedBasePath}` : "";

export function appPath(path: string) {
    const value = path.trim();
    if (!value || /^[a-z][a-z\d+.-]*:\/\//i.test(value) || value.startsWith("//")) {
        return value;
    }
    const normalized = value.startsWith("/") ? value : `/${value}`;
    if (!APP_BASE_PATH || normalized === APP_BASE_PATH || normalized.startsWith(`${APP_BASE_PATH}/`)) {
        return normalized;
    }
    return `${APP_BASE_PATH}${normalized}`;
}

export function logicalAppPath(path: string) {
    if (!APP_BASE_PATH) return path;
    if (path === APP_BASE_PATH) return "/";
    return path.startsWith(`${APP_BASE_PATH}/`) ? path.slice(APP_BASE_PATH.length) || "/" : path;
}
