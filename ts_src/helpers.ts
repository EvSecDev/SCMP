// Result type for explicit value/error handling
export type Result<T> =
    | { ok: true; value: T }
    | { ok: false; error: string };

// Server RPC types
type JsonRpcError = {
    code: number;
    message: string;
    data?: string;
};
type JsonRpcResponse<T> = {
    jsonrpc: "2.0";
    result?: T;
    error?: JsonRpcError;
    id: string;
};

const rpcErrorMessages: Record<number, string> = {
    // Standard JSON-RPC 2 codes
    [-32700]: "Parse Error",
    [-32600]: "Invalid Request",
    [-32601]: "Method Not Found",
    [-32602]: "Invalid Params",
    [-32603]: "Internal Error",

    // Custom error codes
    [-32001]: "Unauthorized",
    [-32003]: "Conflict",
    [-32012]: "Invalid State",
};

const httpAPIPath = "/api/";

const repoHeaderKey = "SCMP-Repository";
let currentRepoHeader: string | null = null;

// Helper to quickly check error presence
export function isErr<T>(result: Result<T>): result is { ok: false; error: string } {
    return !result.ok;
}

// Do this now, this file should be called very early on
initWarnErrUI();

async function ensureRepoHeader(): Promise<void> {
    // Check if we already have a repo set in memory
    if (currentRepoHeader) return;

    // Check localStorage
    const storedRepo = localStorage.getItem("selectedRepo");
    if (storedRepo) {
        currentRepoHeader = storedRepo;
        return;
    }

    // Repo not found — fetch dropdown & repo list
    // Import dynamically to avoid circular dependency if needed
    if (typeof initRepoDropdown === "function") {
        await initRepoDropdown();
    }

    // After init, currentRepoHeader should be set
    if (!currentRepoHeader) {
        console.warn("No repository selected after initRepoDropdown");
    }
}

// Strips JSONRPC wrapper to return either error message or result object
export function parseApiResponse<T = any>(data: unknown): Result<T> {
    if (!data || typeof data !== "object") {
        return { ok: false, error: "Invalid response format" };
    }

    const resp = data as JsonRpcResponse<T>;

    // Validate JSON-RPC version
    if (resp.jsonrpc !== "2.0") {
        return { ok: false, error: "Invalid JSON-RPC version" };
    }

    // If error is present, parse it
    if (resp.error) {
        const err = resp.error;

        const translated = rpcErrorMessages[err.code] || `UnknownCode(${err.code})`;

        let errorText = `${translated}: ${err.message}`;
        if (err.data) {
            errorText += `: ${err.data}`;
        }

        return { ok: false, error: errorText };
    }

    // If result is present, return it
    if ("result" in resp) {
        return { ok: true, value: resp.result as T };
    }

    // Neither result nor error is present
    return { ok: false, error: "Response missing 'result' and 'error'" };
}

// JSONRPC wrapper and API query entry point
export async function getJSONViaJSON<reqJSON = unknown, respJSON = any>(rpcMethod: string, payload?: reqJSON,): Promise<Result<respJSON>> {
    await ensureRepoHeader();

    let response: Response;

    // JSON-RPC request body structure
    const rpcRequest = {
        jsonrpc: "2.0",
        method: rpcMethod,
        params: payload ?? {},
        id: "1",
    };

    const headers: Record<string, string> = {
        "Content-Type": "application/json",
        "Accept": "application/json"
    };

    if (currentRepoHeader) {
        headers[repoHeaderKey] = currentRepoHeader;
    }

    try {
        response = await fetch(httpAPIPath, {
            method: "POST",
            headers,
            body: JSON.stringify(rpcRequest),
        });
    } catch (err: any) {
        return { ok: false, error: `failed to fetch '${httpAPIPath}': ${err.message}` };
    }

    let rawJSON: unknown = null;
    try {
        rawJSON = await response.json().catch(() => null);
    } catch (_) {
        // ignore parse errors
    }

    if (rawJSON) {
        const parsed = parseApiResponse<respJSON>(rawJSON);
        if (isErr(parsed)) {
            const statusMsg = !response.ok ? `HTTP ${response.status}: ` : "";
            return { ok: false, error: `${statusMsg}${parsed.error}` };
        }

        return parsed;
    }

    if (!response.ok) {
        return { ok: false, error: `HTTP ${response.status}` };
    }

    return { ok: false, error: `response was not a json` };
}

// Sending raw body with an expected JSONRPC response
export async function sendData<RespJSON = any>(
    path: string,
    method: string,
    payload: string,
    expectJsonResponse: boolean = true
): Promise<Result<RespJSON | void>> {
    // Convert string to bytes
    let bytes: Uint8Array<ArrayBuffer>;
    try {
        bytes = new TextEncoder().encode(payload);
    } catch (err: any) {
        return { ok: false, error: `failed to encode payload: ${err.message}` };
    }

    // Send request
    let response: Response;
    try {
        response = await fetch(path, {
            method,
            headers: {
                "Content-Type": "application/octet-stream",
                ...(expectJsonResponse ? { "Accept": "application/json" } : {})
            },
            body: bytes,
        });
    } catch (err: any) {
        return { ok: false, error: `failed to send request to '${path}': ${err.message}` };
    }

    // No JSON expected — just check status
    if (!expectJsonResponse) {
        if (!response.ok) {
            return { ok: false, error: `HTTP ${response.status}` };
        }
        return { ok: true, value: undefined };
    }

    // Try to parse the body as JSON
    let rawJSON: unknown = null;
    try {
        rawJSON = await response.json().catch(() => null);
    } catch (_) {
        rawJSON = null;
    }

    // If valid JSON, try parsing API response structure
    if (rawJSON) {
        const parsed = parseApiResponse<RespJSON>(rawJSON);
        if (isErr(parsed)) {
            const statusMsg = !response.ok ? `HTTP ${response.status}: ` : "";
            return { ok: false, error: `${statusMsg}${parsed.error}` };
        }
        return parsed;
    }

    // No JSON, but error status
    if (!response.ok) {
        return { ok: false, error: `HTTP ${response.status}` };
    }

    // No JSON but status was OK — treat as void
    return { ok: true, value: undefined };
}

/* ==================== PERSISTENT USER REQUEST INFO ==================== */

export async function initRepoDropdown() {
    const repoSelect = document.getElementById("repo-select") as HTMLSelectElement;

    // Key names for localStorage
    const STORAGE_SELECTED_REPO = "selectedRepo";
    const STORAGE_ALL_REPOS = "allRepos";

    interface RepoListResp {
        repositories: string[];
    }

    // Try to load selected repo from localStorage
    let selectedRepo = localStorage.getItem(STORAGE_SELECTED_REPO);
    let allRepos: string[] = JSON.parse(localStorage.getItem(STORAGE_ALL_REPOS) || "[]");

    // If no cached repos, fetch them
    if (allRepos.length === 0) {
        const result = await getJSONViaJSON<null, RepoListResp>("settings.repositories.list");
        if (isErr(result)) {
            logError(`Failed to load repo list: ${result.error}`);
            repoSelect.innerHTML = `<option value="">Error loading repos</option>`;
            return;
        }
        allRepos = result.value.repositories ?? [];
        localStorage.setItem(STORAGE_ALL_REPOS, JSON.stringify(allRepos));
    }

    // Populate dropdown
    repoSelect.innerHTML = "";
    allRepos.forEach(repo => {
        const opt = document.createElement("option");
        opt.value = repo;
        opt.textContent = repo;
        repoSelect.appendChild(opt);
    });

    // Set selected repo if available, else default to first
    if (!selectedRepo && allRepos.length > 0) {
        selectedRepo = allRepos[0];
        localStorage.setItem(STORAGE_SELECTED_REPO, selectedRepo);
    }

    if (selectedRepo) {
        repoSelect.value = selectedRepo;
        currentRepoHeader = selectedRepo;
        localStorage.setItem(STORAGE_SELECTED_REPO, selectedRepo);
    }

    // Listen for changes and store selection
    repoSelect.addEventListener("change", () => {
        const newRepo = repoSelect.value;
        localStorage.setItem(STORAGE_SELECTED_REPO, newRepo);
        currentRepoHeader = newRepo;
    });
}

/* ==================== ERROR HANDLING AND UI GENERATION ==================== */
// Initialize UI elements
export function initWarnErrUI() {
    // Popup modal
    if (!document.getElementById("warnerr-modal")) {
        const modal = document.createElement("div");
        modal.id = "warnerr-modal";
        modal.className = "modal hidden";
        modal.innerHTML = `
      <div class="modal-content">
        <p id="warnerr-modal-text"></p>
        <button id="warnerr-modal-ok">OK</button>
      </div>`;
        document.body.appendChild(modal);
    }

    // Notification container
    if (!document.getElementById("notification-container")) {
        const notificationContainer = document.createElement("div");
        notificationContainer.id = "notification-container";
        notificationContainer.className = "notification-container";
        document.body.appendChild(notificationContainer);
    }
}

// Entrance point for all warnings
export function logWarning(message: string) {
    // Log every warning to console
    console.warn(message);
    showWarningNotification(message);
}

function showWarningNotification(message: string, durationMs = 15000) {
    const container = document.getElementById("notification-container")!;
    const notification = document.createElement("div");
    notification.className = "notification warning"; // add "warning" class
    notification.textContent = message;

    container.appendChild(notification);

    setTimeout(() => {
        notification.remove();
    }, durationMs);
}

// Entrance point for all errors
export function logError(message: string, popupConfirm = false) {
    // Log every error to console
    console.error(message);

    if (popupConfirm) {
        showConfirmPopup(message);
    } else {
        showErrNotification(message);
    }
}

// Error Popup with required exit button
function showConfirmPopup(message: string) {
    const modal = document.getElementById("warnerr-modal")!;
    const text = document.getElementById("warnerr-modal-text")!;
    const okBtn = document.getElementById("warnerr-modal-ok")!;

    text.textContent = message;
    modal.classList.remove("hidden");

    okBtn.onclick = () => {
        modal.classList.add("hidden");
    };
}

// Error Popup with auto close
function showErrNotification(message: string, durationMs = 15000) {
    const container = document.getElementById("notification-container")!;
    const notification = document.createElement("div");
    notification.className = "notification";
    notification.textContent = message;

    container.appendChild(notification);

    setTimeout(() => {
        notification.remove();
    }, durationMs);
}

/* ==================== VERSION INFO ==================== */

type VersionInfoResp = {
    fullProgramName: string;
    versionString: string;
    platform: string;
    architecture: string;
    apiBrowserLocation: string;
    docsLink: string;
};

export async function initVersionInfo() {
    const result = await getJSONViaJSON<null, VersionInfoResp>("settings.info.version");
    if (isErr(result)) {
        console.warn(`Failed to load version info: ${result.error}`);
        return;
    }

    const info = result.value;

    const versionEl = document.getElementById("version-info");
    if (versionEl) {
        versionEl.textContent = `${info.fullProgramName} ${info.versionString}`;
    }

    const platformEl = document.getElementById("platform-info");
    if (platformEl) {
        platformEl.textContent = `Platform: ${info.platform} ${info.architecture}`;
    }

    const apiBrowserLink = document.querySelector("#api-browser a") as HTMLAnchorElement | null;
    if (apiBrowserLink && info.apiBrowserLocation) {
        apiBrowserLink.href = info.apiBrowserLocation;
    }

    const githubLink = document.querySelector("#github-link a") as HTMLAnchorElement | null;
    if (githubLink && info.docsLink) {
        githubLink.href = info.docsLink;
    }
}