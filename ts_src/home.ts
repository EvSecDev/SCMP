import { logError, isErr, getJSONViaJSON, initRepoDropdown, initVersionInfo } from "./helpers.js";

function getClaimFromJWT(cookieName: string, claim: string): string | null {
    const cookie = document.cookie
        .split('; ')
        .find(row => row.startsWith(`${cookieName}=`));
    if (!cookie) return null;

    const token = cookie.split('=')[1];
    const parts = token.split('.');
    if (parts.length !== 3) return null;

    try {
        const payload = JSON.parse(atob(parts[1]));
        return typeof payload[claim] === 'string' ? payload[claim] : null;
    } catch (err) {
        console.error("Failed to parse JWT token from cookie", err);
        return null;
    }
}

async function checkConnection(): Promise<void> {
    const connStatusEl = document.getElementById("conn-status");
    if (!connStatusEl) return;

    const statusColors = {
        connected: "lightgreen",
        disconnected: "orange",
        error: "red",
    };

    try {
        const result = await fetch("/health", { method: "GET" });
        if (!result.ok) {
            connStatusEl.textContent = "Unhealthy";
            connStatusEl.style.color = statusColors.disconnected;
        }

        connStatusEl.textContent = "Connected";
        connStatusEl.style.color = statusColors.connected;
    } catch (err) {
        connStatusEl.textContent = "Disconnected";
        connStatusEl.style.color = statusColors.error;
    }
}

async function checkGitStatus(): Promise<void> {
    const gitStatusEl = document.getElementById("git-status");
    if (!gitStatusEl) return;

    type WebRepoStatus = {
        staged: WebRepoFileStatus[];
        unstaged: WebRepoFileStatus[];
    };

    type WebRepoFileStatus = {
        path: string;
        status: string;
    };

    const result = await getJSONViaJSON<WebRepoStatus>("repo.staging.status");
    if (isErr(result)) {
        logError(`Failed to load repo status: ${result.error}`);
        return;
    }
    const status = result.value;
    const totalStaged: number = status.staged.length;
    const totalUnstaged: number = status.unstaged.length;

    gitStatusEl.textContent = `${totalUnstaged} unstaged file(s) / ${totalStaged} staged file(s)`;
}

document.addEventListener("DOMContentLoaded", () => {
    initVersionInfo();
    initRepoDropdown();

    // Username from JWT
    const userName = getClaimFromJWT("id_token", "name");
    if (userName) {
        const nameEl = document.getElementById("user-name");
        if (nameEl) {
            nameEl.textContent = userName;
        }
    }

    // Repo status (one time)
    checkGitStatus();

    // Initial check and polling
    checkConnection();
    setInterval(checkConnection, 30000);
});
