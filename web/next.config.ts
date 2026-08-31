import type { NextConfig } from "next";
import { PHASE_DEVELOPMENT_SERVER } from "next/constants";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";
import { parseChangelog } from "@/lib/release";

const webDir = dirname(fileURLToPath(import.meta.url));
const localVersion = readFileSync(resolve(webDir, "../VERSION"), "utf8").trim() || "dev";
const localChangelog = readFileSync(resolve(webDir, "../CHANGELOG.md"), "utf8");

export default function nextConfig(phase: string): NextConfig {
    const isDev = phase === PHASE_DEVELOPMENT_SERVER;
    const releases = parseChangelog(localChangelog);
    const configuredBasePath = process.env.NEXT_PUBLIC_BASE_PATH?.trim() || "";
    const basePath = configuredBasePath && configuredBasePath !== "/"
        ? `/${configuredBasePath.replace(/^\/+|\/+$/g, "")}`
        : "";

    return {
        output: "standalone",
        basePath,
        allowedDevOrigins: isDev ? ["*.*.*.*"] : [],
        typescript: {
            ignoreBuildErrors: true,
        },
        env: {
            NEXT_PUBLIC_BASE_PATH: basePath,
            NEXT_PUBLIC_FANREN_SSO: process.env.NEXT_PUBLIC_FANREN_SSO || "false",
            NEXT_PUBLIC_APP_VERSION: localVersion,
            NEXT_PUBLIC_APP_RELEASES: JSON.stringify(releases),
        },
    };
}
