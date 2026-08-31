import type { AuthUser } from "@/services/api/auth";

export type FanrenAccountKey = {
    id: number;
    name: string;
    key: string;
    status: number;
    group: string;
    remain_quota: number;
    used_quota: number;
    unlimited_quota: boolean;
    expired_time: number;
    routing_policy_id: string;
    routing_groups: string;
};

type FanrenEnvelope<T> = {
    success?: boolean;
    message?: string;
    data?: T;
};

type FanrenAuthPayload = {
    access_token: string;
    token_type: string;
    access_expires_at: number;
    user: Record<string, unknown>;
};

function mainApiHeaders(token?: string, contentType = "") {
    return {
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...(contentType ? { "Content-Type": contentType } : {}),
        "Cache-Control": "no-store",
    };
}

function mapFanrenUser(value: Record<string, unknown>): AuthUser {
    const id = Number(value.id || 0);
    const role = Number(value.role || 0);
    return {
        id: String(id),
        username: String(value.username || ""),
        displayName: String(value.display_name || value.username || ""),
        avatarUrl: "",
        role: role >= 10 ? "admin" : "user",
        credits: Number(value.quota || 0),
        createdAt: String(value.created_at || ""),
        updatedAt: "",
    };
}

export async function refreshFanrenSession() {
    const response = await fetch("/api/user/auth/refresh", {
        method: "POST",
        credentials: "include",
        headers: mainApiHeaders(undefined, "application/json"),
        body: "{}",
    });
    if (!response.ok) return null;
    const payload = (await response.json()) as FanrenEnvelope<FanrenAuthPayload>;
    if (!payload.success || !payload.data?.access_token || !payload.data.user) return null;
    return {
        token: payload.data.access_token,
        user: mapFanrenUser(payload.data.user),
    };
}

export async function fetchFanrenCurrentUser(token: string) {
    const response = await fetch("/api/user/self", { headers: mainApiHeaders(token), credentials: "include" });
    if (!response.ok) throw new Error("凡人登录会话已失效");
    const payload = (await response.json()) as FanrenEnvelope<Record<string, unknown>>;
    if (!payload.success || !payload.data) throw new Error(payload.message || "无法读取凡人账号");
    return mapFanrenUser(payload.data);
}

export async function fetchFanrenAccountKeys(token: string) {
    const response = await fetch("/api/token/?p=1&size=100", { headers: mainApiHeaders(token), credentials: "include" });
    if (!response.ok) throw new Error("无法读取凡人 Key");
    const payload = (await response.json()) as FanrenEnvelope<{ items?: FanrenAccountKey[] }>;
    if (!payload.success || !payload.data?.items) throw new Error(payload.message || "无法读取凡人 Key");
    return payload.data.items;
}

export async function fetchFanrenModels(token: string) {
    const response = await fetch("/api/user/models", { headers: mainApiHeaders(token), credentials: "include" });
    if (!response.ok) throw new Error("无法读取凡人模型");
    const payload = (await response.json()) as FanrenEnvelope<string[]>;
    if (!payload.success || !Array.isArray(payload.data)) throw new Error(payload.message || "无法读取凡人模型");
    return payload.data;
}

export async function logoutFanrenSession(token: string) {
    await fetch("/api/user/auth/logout", {
        method: "POST",
        credentials: "include",
        headers: mainApiHeaders(token, "application/json"),
        body: "{}",
    }).catch(() => undefined);
}
