import { isErr } from "../result.js"
import { getJSONViaJSON } from "../rpc/client.js"
import { logWarning } from "../logging/log.js"
import { initRepoDropdown } from "../../ui/dropdown.js"
import type { VersionInfo } from "../../types/settings.js"

// Combines initVersionInfo + initRepoDropdown - call from DOMContentLoaded on every page
export async function initPage() {
    await initVersionInfo()
    await initRepoDropdown()
}

export async function initVersionInfo() {
    const result = await getJSONViaJSON<null, VersionInfo>("settings.info.version")
    if (isErr(result)) {
        logWarning(`initVersionInfo: ${result.error}`)
        return
    }

    const info = result.value

    const versionEl = document.getElementById("version-info")
    if (versionEl) {
        versionEl.textContent = `${info.fullProgramName} ${info.versionString}`
    }

    const platformEl = document.getElementById("platform-info")
    if (platformEl) {
        platformEl.textContent = `Platform: ${info.platform} ${info.architecture}`
    }

    const apiBrowserLink = document.querySelector("#api-browser a")
    if (apiBrowserLink && info.apiBrowserLocation) {
        if (apiBrowserLink instanceof HTMLAnchorElement) {
            apiBrowserLink.href = info.apiBrowserLocation
        }
    }

    const githubLink = document.querySelector("#github-link a")
    if (githubLink && info.docsLink) {
        if (githubLink instanceof HTMLAnchorElement) {
            githubLink.href = info.docsLink
        }
    }
}
