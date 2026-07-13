import { isErr } from "./lib/result.js"
import { doFetch } from "./lib/http/fetch.js"
import { parseJSON } from "./lib/parse/json.js"
import { safeAtob } from "./lib/parse/encoding.js"
import { getJSONViaJSON } from "./lib/rpc/client.js"
import { logError, logWarning } from "./lib/logging/log.js"
import { initPage } from "./lib/init/page.js"
import type { RepoStatus } from "./types/repository.js"

function getClaimFromJWT(cookieName: string, claim: string): string | null {
    var result: string | null
    var rows = document.cookie.split("; ")
    for (var rowIndex = 0; rowIndex < rows.length; rowIndex++) {
        const row = rows[rowIndex]
        if (row == null) {
            continue
        }
        if (row.startsWith(`${cookieName}=`)) {
            const token = decodeURIComponent(row.slice(cookieName.length + 1))
            const parts = token.split(".")
            if (parts.length !== 3) {
                return null
            }

            const part1 = parts[1]
            if (part1 == null) {
                return null
            }
            var decodeResult = safeAtob(part1)
            if (isErr(decodeResult)) {
                logError(`getClaimFromJWT: decode JWT: ${decodeResult.error}`, false)
                return null
            }

            var parseResult = parseJSON<Record<string, unknown>>(decodeResult.value)
            if (isErr(parseResult)) {
                logError(`getClaimFromJWT: parse JWT: ${parseResult.error}`, false)
                return null
            }

            var value = parseResult.value[claim]
            if (typeof value === "string") {
                result = value
                return result
            }

            return null
        }
    }
    result = null
    return result
}

async function checkConnection(): Promise<void> {
    var connStatusEl = document.getElementById("conn-status")
    if (!connStatusEl) {
        return
    }

    var fetchResult = await doFetch("/health", { method: "GET" })
    if (isErr(fetchResult)) {
        connStatusEl.textContent = "Disconnected"
        connStatusEl.style.color = "red"
        logWarning(`checkConnection: ${fetchResult.error}`)
        return
    }

    var response = fetchResult.value
    if (!response.ok) {
        connStatusEl.textContent = "Unhealthy"
        connStatusEl.style.color = "orange"
        return
    }

    connStatusEl.textContent = "Connected"
    connStatusEl.style.color = "lightgreen"
}

async function checkGitStatus(): Promise<void> {
    var gitStatusEl = document.getElementById("git-status")
    if (!gitStatusEl) {
        return
    }

    var result = await getJSONViaJSON<RepoStatus>("repo.staging.status")
    if (isErr(result)) {
        logError(`checkGitStatus: ${result.error}`, false)
        return
    }

    var status = result.value
    var totalStaged = status.staged.length
    var totalUnstaged = status.unstaged.length

    gitStatusEl.textContent = `${totalUnstaged} unstaged file(s) / ${totalStaged} staged file(s)`
}

document.addEventListener("DOMContentLoaded", () => {
    initPage()

    var userName = getClaimFromJWT("id_token", "name")
    if (userName) {
        var nameEl = document.getElementById("user-name")
        if (nameEl) {
            nameEl.textContent = userName
        }
    }

    checkGitStatus()

    checkConnection()
    setInterval(checkConnection, 30000)
})
