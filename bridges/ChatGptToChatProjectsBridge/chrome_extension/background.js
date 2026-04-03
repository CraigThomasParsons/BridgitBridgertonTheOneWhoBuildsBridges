// background.js
// ChatProjects Token Refresher Background Service Worker
// Periodically extracts ChatGPT session token and sends it to the bridge API

const BRIDGE_API_URL = "http://localhost:8000/api/token/update";
const POLL_INTERVAL_MINUTES = 1; // Check every 1 minute (adjust as needed)

console.log("[ChatProjects Token Refresher] Service worker loaded");

// Create alarm to periodically refresh token
chrome.alarms.create("refreshToken", { periodInMinutes: POLL_INTERVAL_MINUTES });

chrome.alarms.onAlarm.addListener(async (alarm) => {
    if (alarm.name === "refreshToken") {
        console.log("[ChatProjects Token Refresher] Alarm triggered, extracting token...");
        await refreshToken();
    }
});

/**
 * Extracts ChatGPT session token from cookies and POSTs it to the bridge API
 */
async function refreshToken() {
    try {
        // Search across all domains ChatGPT might use for auth cookies
        const domains = ["chatgpt.com", ".chatgpt.com", "openai.com", ".openai.com", "auth0.openai.com"];
        let allCookies = [];

        for (const domain of domains) {
            const cookies = await chrome.cookies.getAll({ domain });
            if (cookies.length > 0) {
                console.log(`[ChatProjects Token Refresher] Cookies on ${domain}:`, cookies.map(c => c.name));
                allCookies = allCookies.concat(cookies);
            }
        }

        // Also try URL-based lookup (sometimes domain-based misses cookies)
        for (const url of ["https://chatgpt.com/", "https://auth.openai.com/", "https://auth0.openai.com/"]) {
            const cookies = await chrome.cookies.getAll({ url });
            if (cookies.length > 0) {
                console.log(`[ChatProjects Token Refresher] Cookies at ${url}:`, cookies.map(c => c.name));
                allCookies = allCookies.concat(cookies);
            }
        }

        // Deduplicate by name+domain
        const seen = new Set();
        allCookies = allCookies.filter(c => {
            const key = `${c.name}@${c.domain}`;
            if (seen.has(key)) return false;
            seen.add(key);
            return true;
        });

        console.log(`[ChatProjects Token Refresher] Total unique cookies found: ${allCookies.length}`);

        if (allCookies.length === 0) {
            console.warn("[ChatProjects Token Refresher] No cookies found on any ChatGPT/OpenAI domain. Are you logged in?");
            return;
        }

        // Try known session token cookie names
        const cookieNames = [
            "__Secure-next-auth.session-token",
        ];

        let tokenValue = null;

        // Direct name match for non-chunked token
        for (const name of cookieNames) {
            const match = allCookies.find(c => c.name === name);
            if (match && match.value) {
                console.log(`[ChatProjects Token Refresher] Found token in cookie: ${match.name} on ${match.domain} (length: ${match.value.length})`);
                tokenValue = match.value;
                break;
            }
        }

        // If no single token, check for chunked tokens (.0, .1, etc.) and concatenate them
        if (!tokenValue) {
            const chunk0 = allCookies.find(c => c.name === "__Secure-next-auth.session-token.0");
            const chunk1 = allCookies.find(c => c.name === "__Secure-next-auth.session-token.1");
            if (chunk0 && chunk0.value) {
                tokenValue = chunk0.value + (chunk1 ? chunk1.value : "");
                console.log(`[ChatProjects Token Refresher] Assembled chunked token: .0 (${chunk0.value.length}) + .1 (${chunk1 ? chunk1.value.length : 0}) = ${tokenValue.length} total chars`);
            }
        }

        // Fallback: search all cookies for anything session-like
        if (!tokenValue) {
            const sessionCookie = allCookies.find(c =>
                c.name.includes("session") || c.name.includes("token") || c.name.includes("auth")
            );
            if (sessionCookie) {
                console.log(`[ChatProjects Token Refresher] Fallback: found cookie "${sessionCookie.name}" on ${sessionCookie.domain} (length: ${sessionCookie.value.length})`);
                tokenValue = sessionCookie.value;
            }
        }

        if (!tokenValue) {
            console.warn("[ChatProjects Token Refresher] No session token found.");
            console.warn("[ChatProjects Token Refresher] All cookies found:", allCookies.map(c => `${c.name}@${c.domain} (${c.value.length} chars)`));
            return;
        }

        console.log("[ChatProjects Token Refresher] Token found, sending to bridge API...");

        // Send token to bridge API endpoint
        const response = await fetch(BRIDGE_API_URL, {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({
                token: tokenValue,
                device_id: await getDeviceId()
            })
        });

        if (!response.ok) {
            console.error(`[ChatProjects Token Refresher] API error: ${response.status} ${response.statusText}`);
            return;
        }

        const data = await response.json();
        console.log("[ChatProjects Token Refresher] Token updated successfully:", data.message || "OK");

        // Store last update timestamp
        chrome.storage.local.set({
            lastTokenUpdate: new Date().toISOString(),
            lastTokenLength: tokenValue.length
        });

    } catch (error) {
        console.error("[ChatProjects Token Refresher] Error:", error);
    }
}

/**
 * Gets or generates a stable device ID for this extension
 */
async function getDeviceId() {
    return new Promise((resolve) => {
        chrome.storage.local.get("deviceId", (result) => {
            if (result.deviceId) {
                resolve(result.deviceId);
            } else {
                // Generate UUID-like device ID
                const deviceId = generateUUID();
                chrome.storage.local.set({ deviceId });
                resolve(deviceId);
            }
        });
    });
}

/**
 * Simple UUID v4 generator
 */
function generateUUID() {
    return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, function (c) {
        const r = Math.random() * 16 | 0;
        const v = c === "x" ? r : (r & 0x3 | 0x8);
        return v.toString(16);
    });
}

// Try to refresh immediately on startup (in case the token is stale)
refreshToken().catch(err => console.warn("[ChatProjects Token Refresher] Startup refresh failed:", err));
