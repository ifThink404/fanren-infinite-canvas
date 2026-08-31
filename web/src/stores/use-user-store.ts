"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";

import { AUTH_TOKEN_KEY, fetchCurrentUser, login, register, type AuthPayload, type AuthUser } from "@/services/api/auth";
import { fetchFanrenCurrentUser, logoutFanrenSession, refreshFanrenSession } from "@/services/api/fanren-account";
import { FANREN_SSO_ENABLED } from "@/lib/fanren";

type UserStore = {
    token: string;
    user: AuthUser | null;
    isReady: boolean;
    isLoading: boolean;
    setSession: (token: string, user: AuthUser) => void;
    clearSession: () => void;
    hydrateUser: () => Promise<void>;
    signInWithFanren: () => Promise<AuthUser>;
    login: (payload: AuthPayload) => Promise<AuthUser>;
    register: (payload: AuthPayload) => Promise<AuthUser>;
};

export const useUserStore = create<UserStore>()(
    persist(
        (set, get) => ({
            token: "",
            user: null,
            isReady: false,
            isLoading: false,
            setSession: (token, user) => set({ token, user, isReady: true }),
            clearSession: () => {
                const token = get().token;
                set({ token: "", user: null, isReady: true });
                if (FANREN_SSO_ENABLED && token) void logoutFanrenSession(token);
            },
            hydrateUser: async () => {
                const token = get().token;
                if (!token) {
                    if (FANREN_SSO_ENABLED) {
                        try {
                            const session = await refreshFanrenSession();
                            if (session) {
                                set({ token: session.token, user: session.user, isReady: true, isLoading: false });
                                return;
                            }
                        } catch {
                            // Fall through to the anonymous state.
                        }
                    }
                    set({ user: null, isReady: true, isLoading: false });
                    return;
                }
                set({ isLoading: true });
                try {
                    const user = FANREN_SSO_ENABLED ? await fetchFanrenCurrentUser(token) : await fetchCurrentUser(token);
                    if (user.role === "guest") {
                        set({ token: "", user: null, isReady: true, isLoading: false });
                        return;
                    }
                    set({ user, isReady: true, isLoading: false });
                } catch {
                    if (FANREN_SSO_ENABLED) {
                        try {
                            const session = await refreshFanrenSession();
                            if (session) {
                                set({ token: session.token, user: session.user, isReady: true, isLoading: false });
                                return;
                            }
                        } catch {
                            // Fall through to the signed-out state.
                        }
                    }
                    set({ token: "", user: null, isReady: true, isLoading: false });
                }
            },
            signInWithFanren: async () => {
                set({ isLoading: true });
                try {
                    const session = await refreshFanrenSession();
                    if (!session) throw new Error("请先登录凡人站账号");
                    set({ token: session.token, user: session.user, isReady: true, isLoading: false });
                    return session.user;
                } catch (error) {
                    set({ isLoading: false });
                    throw error;
                }
            },
            login: async (payload) => {
                set({ isLoading: true });
                try {
                    const session = await login(payload);
                    set({ token: session.token, user: session.user, isReady: true, isLoading: false });
                    return session.user;
                } catch (error) {
                    set({ isLoading: false });
                    throw error;
                }
            },
            register: async (payload) => {
                set({ isLoading: true });
                try {
                    const session = await register(payload);
                    set({ token: session.token, user: session.user, isReady: true, isLoading: false });
                    return session.user;
                } catch (error) {
                    set({ isLoading: false });
                    throw error;
                }
            },
        }),
        {
            name: AUTH_TOKEN_KEY,
            partialize: (state) => ({ token: state.token }),
            onRehydrateStorage: () => (state) => {
                if (state) state.isReady = false;
            },
        },
    ),
);
