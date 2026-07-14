import { isErr, id } from "../lib/result.js"
import { mustElement } from "../lib/dom/lookup.js"
import { filterElementsByText } from "../lib/dom/filter.js"
import { logError } from "../lib/logging/log.js"
import { parseJSON } from "../lib/parse/json.js"
import { getJSONViaJSON } from "../lib/rpc/client.js"
import { setCurrentRepoHeader } from "../lib/rpc/auth.js"
import type { RepoList, HostList } from "../types/settings.js"

export async function initRepoDropdown() {
    const selectResult = mustElement<HTMLSelectElement>(id("repo-select"))
    if (isErr(selectResult)) {
        return;
    }
    const repoSelect = selectResult.value

    const STORAGE_SELECTED_REPO = "selectedRepo"
    const STORAGE_ALL_REPOS = "allRepos"

    let selectedRepo = localStorage.getItem(STORAGE_SELECTED_REPO)
    const storedAllRepos = localStorage.getItem(STORAGE_ALL_REPOS)
    var allRepos: string[] = []
    if (storedAllRepos) {
        const parseResult = parseJSON<string[]>(storedAllRepos)
        if (parseResult.ok) {
            allRepos = parseResult.value
        }
    }

    if (allRepos.length === 0) {
        const result = await getJSONViaJSON<null, RepoList>("settings.repositories.list")
        if (isErr(result)) {
            logError(`initRepoDropdown: load repos: ${result.error}`, false)
            repoSelect.innerHTML = "<option value=\"\">Error loading repos</option>"
            return
        }
        allRepos = result.value.repositories
        if (!allRepos) {
            allRepos = []
        }
        localStorage.setItem(STORAGE_ALL_REPOS, JSON.stringify(allRepos))
    }

    repoSelect.innerHTML = ""
    for (let i = 0; i < allRepos.length; i++) {
        const r = allRepos[i];
        if (r == null) continue;
        const opt = document.createElement("option")
        opt.value = r
        opt.textContent = r
        repoSelect.appendChild(opt)
    }

    if (!selectedRepo && allRepos.length > 0) {
        const firstRepo = allRepos[0];
        if (firstRepo == null) return;
        selectedRepo = firstRepo
        localStorage.setItem(STORAGE_SELECTED_REPO, selectedRepo)
    }

    if (selectedRepo) {
        repoSelect.value = selectedRepo
        setCurrentRepoHeader(selectedRepo)
        localStorage.setItem(STORAGE_SELECTED_REPO, selectedRepo)
    }

    repoSelect.addEventListener("change", () => {
        const newRepo = repoSelect.value
        localStorage.setItem(STORAGE_SELECTED_REPO, newRepo)
        setCurrentRepoHeader(newRepo)
    })
}

export const selectedHosts: Set<string> = new Set()
let hostsDropdownSetup = false
let hostsClickHandler: ((e: MouseEvent) => void) | null = null

export function resetHostsDropdownState() {
    selectedHosts.clear()
    hostsDropdownSetup = false
}

export async function setupOverrideHostsDropdown() {
    if (hostsDropdownSetup) return
    hostsDropdownSetup = true

    // Clear stale state from previous page load / refresh
    selectedHosts.clear()

    const inputResult = mustElement<HTMLInputElement>(id("overridehosts-input"))
    if (isErr(inputResult)) {
        logError(`setupOverrideHostsDropdown: ${inputResult.error}`, false)
        return
    }
    const inputEl = inputResult.value
    inputEl.value = ""
    const dropdownResult = mustElement<HTMLDivElement>(id("overridehosts-dropdown"))
    if (isErr(dropdownResult)) {
        logError(`setupOverrideHostsDropdown: ${dropdownResult.error}`, false)
        return
    }
    const dropdownEl = dropdownResult.value

    const result = await getJSONViaJSON<{ withDetails: boolean }, HostList>("settings.hosts.list", { withDetails: false })

    dropdownEl.innerHTML = ""
    if (isErr(result)) {
        logError(`setupOverrideHostsDropdown: load hosts: ${result.error}`, false)
        dropdownEl.innerHTML = "<div class=\"dropdown-loading\">Error loading hosts</div>"
        return
    }

    let hosts: string[] = []
    if (result.value && result.value.hosts) {
        hosts = result.value.hosts
    }
    if (hosts.length === 0) {
        dropdownEl.innerHTML = "<div class=\"dropdown-loading\">No hosts available</div>"
        return
    }

    var searchRow = document.createElement("div")
    searchRow.className = "host-row search-row"
    var searchInput = document.createElement("input")
    searchInput.type = "text"
    searchInput.placeholder = "Search..."
    searchRow.appendChild(searchInput)
    dropdownEl.appendChild(searchRow)

    var rowElements: HTMLDivElement[] = []
    for (var hostIndex = 0; hostIndex < hosts.length; hostIndex++) {
        const hostItem = hosts[hostIndex];
        if (hostItem == null) {
            continue
        }
        const host = hostItem;
        const row = document.createElement("div") as HTMLDivElement
        row.className = "host-row"
        row.textContent = host

        row.addEventListener("click", () => {
            if (selectedHosts.has(host)) {
                selectedHosts.delete(host)
                row.classList.remove("selected")
            } else {
                selectedHosts.add(host)
                row.classList.add("selected")
            }
            updateInputValue()
        })

        dropdownEl.appendChild(row)
        rowElements.push(row)
    }

    function updateInputValue() {
        inputEl.value = Array.from(selectedHosts).join(", ")
    }

    searchInput.addEventListener("input", () => {
        filterElementsByText(rowElements, searchInput.value)
    })

    dropdownEl.classList.add("hidden")

    inputEl.addEventListener("click", (e) => {
        e.stopPropagation()
        if (!e.isTrusted) {
            return
        }
        dropdownEl.classList.toggle("hidden")

        if (!dropdownEl.classList.contains("hidden")) {
            searchInput.focus()
            searchInput.select()
        }
    })

    dropdownEl.addEventListener("click", (e) => {
        e.stopPropagation()
    })

    hostsClickHandler = () => {
        dropdownEl.classList.add("hidden")
    }
    document.addEventListener("click", hostsClickHandler)
}
