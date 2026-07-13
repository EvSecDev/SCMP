import { logWarning } from "../logging/log.js"
import { isErr } from "../result.js"
import { fetchRPC, parseRpcResponse } from "./transport.js"

// JSON result type for repository list
interface RepoList {
    repositories: string[]
}

export const httpAPIPath = "/api/"
export const repoHeaderKey = "SCMP-Repository"
let currentRepoHeader: string | null = null

export function getCurrentRepoHeader(): string | null {
    return currentRepoHeader
}

export function setCurrentRepoHeader(value: string | null): void {
    currentRepoHeader = value
}

// Fetch the first available repository from the server.
// Does not rely on UI components, breaking the circular import chain.
async function initFirstRepo(): Promise<void> {
    var rpcRequest: Record<string, unknown> = {
        jsonrpc: "2.0",
        method: "settings.repositories.list",
        params: {},
        id: String(Date.now()),
    }

    var response = await fetchRPC(httpAPIPath, rpcRequest, {})
    if (!response.ok) {
        logWarning("initFirstRepo: fetch failed: " + response.error)
        return
    }

    var parsedResult = parseRpcResponse<RepoList>(response.data, response.status)
    if (isErr(parsedResult)) {
        logWarning("initFirstRepo: parse failed: " + parsedResult.error)
        return
    }

    var repos = parsedResult.value.repositories
    if (!repos || repos.length === 0) {
        logWarning("initFirstRepo: no repositories found")
        return
    }

    const STORAGE_SELECTED_REPO = "selectedRepo"
    const STORAGE_ALL_REPOS = "allRepos"

    var firstRepo = repos[0];
    if (firstRepo == null) return;

    currentRepoHeader = firstRepo
    localStorage.setItem(STORAGE_SELECTED_REPO, firstRepo)
    localStorage.setItem(STORAGE_ALL_REPOS, JSON.stringify(repos))
}

export async function ensureRepoHeader(): Promise<void> {
    if (currentRepoHeader) return;

    var storedRepo = localStorage.getItem("selectedRepo");
    if (storedRepo) {
        currentRepoHeader = storedRepo;
        return;
    }
    await initFirstRepo();

    if (!currentRepoHeader) {
        logWarning("No repository selected after initFirstRepo");
    }
}