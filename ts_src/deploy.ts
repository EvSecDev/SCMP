import { isErr } from "./lib/result.js"
import type { Result } from "./lib/result.js"
import { getElement } from "./lib/dom/lookup.js"
import { getJSONViaJSON } from "./lib/rpc/client.js"
import { logError, logWarning } from "./lib/logging/log.js"
import { initPage } from "./lib/init/page.js"
import { readCheckboxValue, readInputValue, parseIntOrZero } from "./lib/dom/form.js"
import { setupOverrideHostsDropdown, selectedHosts, resetHostsDropdownState } from "./ui/dropdown.js"
import { showModal } from "./ui/modal.js"
import type { DeployStart, DeployStatus, DeployAbort, PromptReq, PromptAnswer, DeployOutput } from "./types/deployment.js"
import type { NilSuccess } from "./types/common.js"

function escapeHtml(s: string): string {
    return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;")
}

function resetDeploymentSummary() {
    var summaryEl = document.getElementById("deploy-summary")
    if (summaryEl) {
        summaryEl.classList.add("hidden")
    }

    var outputEl = getElement("deploy-output") as HTMLElement | null
    if (outputEl) {
        outputEl.textContent = "Loading deployment output..."
    }
}

async function handleDeployClick() {
    var result: Result<void>
    var deployBtn = getElement("deploy-btn") as HTMLButtonElement | null
    if (!deployBtn) {
        result = { ok: false, error: "handleDeploy: deploy-btn not found" }
        return result
    }

    var spinner = getElement("deploy-spinner") as HTMLDivElement | null
    if (!spinner) {
        result = { ok: false, error: "handleDeploy: deploy-spinner not found" }
        return result
    }

    deployBtn.disabled = true
    spinner.classList.remove("hidden")

    // Reset stale summary from previous deployment to initial page-load state
    resetDeploymentSummary()

    var errorOccurred = false

    try {
        // Step 1: Read inputs
        var modeInput = document.querySelector<HTMLInputElement>("input[name='deploy-mode']:checked")
        var mode = "diff"
        if (modeInput) {
            mode = modeInput.id.replace("mode-", "")
        }

        var typeInput = document.querySelector<HTMLInputElement>("input[name='opt-type']:checked")
        var type = "preview"
        if (typeInput) {
            type = typeInput.id.replace("type-", "")
        }

        // Step 2: Construct deployment options
        var opts: DeployStart["options"] = {
            allowDeletions: readCheckboxValue("opt-allow-deletions"),
            runInstall: readCheckboxValue("opt-run-install"),
            disableReloads: readCheckboxValue("opt-disable-reloads"),
            disableSudo: readCheckboxValue("opt-disable-sudo"),
            ignoreHostState: readCheckboxValue("opt-ignore-state"),
            force: readCheckboxValue("opt-force"),
            autoCommitRollbackEnabled: readCheckboxValue("opt-autorollback"),
            commitID: readInputValue("commitid"),
            fileOverride: readInputValue("overridefiles"),
            hostOverride: Array.from(selectedHosts).join(","),
            runAsUser: readInputValue("runasuser"),
            maxSSHConnections: parseIntOrZero(readInputValue("maxsshconns")),
            maxSSHChannels: parseIntOrZero(readInputValue("maxsshchan")),
            maxCommandRuntime: parseIntOrZero(readInputValue("cmdtimeout")),
            verbosity: parseIntOrZero(readInputValue("verbosity"))
        }

        // Step 3: Construct payload
        var payload: DeployStart = {
            mode: mode,
            type: type,
            options: opts
        }

        // Step 4: Send POST request
        var deployRes = await getJSONViaJSON<DeployStart, DeployStatus>("deployment.start", payload)

        if (isErr(deployRes)) {
            logError(`handleDeploy: start: ${deployRes.error}`, false)
            errorOccurred = true
        } else {
            var deploymentID = deployRes.value.deploymentID

            // Step 5: Poll for status
            var pollResult = await pollDeploymentStatus(deploymentID)
            if (isErr(pollResult)) {
                errorOccurred = true
            }
        }
    } finally {
        deployBtn.disabled = false
        spinner.classList.add("hidden")
    }

    if (errorOccurred) {
        result = { ok: false, error: "handleDeploy: deployment failed" }
    } else {
        result = { ok: true, value: undefined }
    }
    return result
}

async function pollDeploymentStatus(reqID: string): Promise<Result<void>> {
    var result: Result<void>
    var abortBtn = getElement("deploy-abort-btn") as HTMLButtonElement | null
    if (!abortBtn) {
        result = { ok: false, error: "pollDeploymentStatus: deploy-abort-btn not found" }
        return result
    }
    var aborted = false
    var abortHandler = async () => {
        await new Promise<void>((resolve) => {
            showModal({
                message: "Are you sure you want to abort this deployment?",
                confirmText: "Abort",
                cancelText: "Cancel",
                onConfirm: async () => {
                    var res = await getJSONViaJSON<DeployAbort, NilSuccess>("deployment.abort", { deploymentID: reqID, stopRequested: true })

                    if (isErr(res)) {
                        logError(`pollDeploymentStatus: abort: ${res.error}`, false)
                        return
                    }
                    aborted = true
                    resolve()
                },
                onCanceled: () => {
                    resolve()
                },
            })
        })
    }

    abortBtn.addEventListener("click", abortHandler)
    abortBtn.removeAttribute("disabled")

    return new Promise<Result<void>>((resolve) => {
        var poll = async () => {
            if (!abortBtn) {
                resolve({ ok: false, error: "pollDeploymentStatus: abortBtn lost" })
                return
            }
            if (aborted) {
                abortBtn.setAttribute("disabled", "")
                abortBtn.removeEventListener("click", abortHandler)
                logWarning(`Aborted deployment ${reqID}`)
                resolve({ ok: true, value: undefined })
                return
            }

            var statusRes = await getJSONViaJSON<{ deploymentID: string }, DeployStatus>("deployment.status", { deploymentID: reqID })

            if (isErr(statusRes)) {
                logError(`pollDeploymentStatus: fetch status ${reqID}: ${statusRes.error}`, false)
                abortBtn.setAttribute("disabled", "")
                abortBtn.removeEventListener("click", abortHandler)
                resolve({ ok: false, error: statusRes.error })
                return
            }

            var status = statusRes.value.status

            if (status === "finished") {
                var outputResult = await fetchDeploymentOutput(reqID)
                abortBtn.setAttribute("disabled", "")
                abortBtn.removeEventListener("click", abortHandler)
                if (isErr(outputResult)) {
                    resolve({ ok: false, error: outputResult.error })
                } else {
                    resolve({ ok: true, value: undefined })
                }
                return
            }

            if (statusRes.value.pending) {
                var pendingAction: PromptReq[] = []
                if (statusRes.value.pendingAction) {
                    pendingAction = statusRes.value.pendingAction
                }
                var err = await askUser(pendingAction)
                if (err.trim() !== "") {
                    logError(`pollDeploymentStatus: prompt answer ${reqID}: ${err}`, false)
                    abortBtn.setAttribute("disabled", "")
                    abortBtn.removeEventListener("click", abortHandler)
                    resolve({ ok: false, error: err })
                    return
            }
        }

        var terminalFailureStatuses = ["failed", "error", "aborted", "cancelled"]
        var isTerminalFailure = false
        for (var idx = 0; idx < terminalFailureStatuses.length; idx++) {
            if (status === terminalFailureStatuses[idx]) {
                isTerminalFailure = true
                break
            }
        }
        if (isTerminalFailure) {
            abortBtn.setAttribute("disabled", "")
            abortBtn.removeEventListener("click", abortHandler)
            resolve({ ok: false, error: `Deployment ended with status: ${status}` })
            return
        }

        setTimeout(poll, 2000)
        }

        poll()
    })
}

async function askUser(prompts: PromptReq[]): Promise<string> {
    const answers: PromptAnswer[] = [];
    let err = "";

    for (const prompt of prompts) {
        let hideInputText: boolean = false;
        if (prompt.type === "secret") {
            hideInputText = true;
        }

        let userInput: string | undefined;
        let cancelled = false;
        await new Promise<void>((resolve) => {
            showModal({
                message: prompt.title,
                inputPlaceholder: "Input",
                confirmText: "Enter",
                cancelText: "Cancel",
                hideInput: hideInputText,
                onConfirm: (inputValue, _checkboxes, _selects, _inputs) => {
                    userInput = inputValue;
                    resolve();
                },
                onCanceled: () => {
                    cancelled = true;
                    resolve();
                },
            });
        });

        if (cancelled) {
            err = `User cancelled prompt: ${prompt.title}`;
            return err;
        }
        if (userInput == undefined) {
            err = `Invalid input for prompt: ${prompt.title}`;
            return err;
        }

        const encoded = btoa(Array.from(new TextEncoder().encode(userInput))
            .map((b) => String.fromCharCode(b)).join(""))
        const newAnswer: PromptAnswer = {
            associatedDataID: prompt.associatedDataID,
            promptID: prompt.promptID,
            encodedData: encoded,
        }
        answers.push(newAnswer);
    }

    const ansResp = await getJSONViaJSON<PromptAnswer[], NilSuccess>("user.pending.prompt.answer", answers);
    if (isErr(ansResp)) {
        err = ansResp.error;
        return err;
    }

    if (ansResp.value.status === "succeeded") {
        return err;
    } else {
        err = "Answering prompts failed: Internal Error";
        return err;
    }
}

async function fetchDeploymentOutput(reqID: string): Promise<Result<void>> {
    var result: Result<void>
    var outputEl = getElement("deploy-output") as HTMLElement | null
    if (!outputEl) {
        result = { ok: false, error: "fetchDeploymentOutput: .deploy-output not found" }
        return result
    }
    var summaryEl = getElement("deploy-summary")
    var outputRes = await getJSONViaJSON<object, DeployOutput>("deployment.result", { deploymentID: reqID })

    if (isErr(outputRes)) {
        logError(`fetchDeploymentOutput: deployment ${reqID}: ${outputRes.error}`, false)
        outputEl.textContent = "(No output)"
        result = { ok: false, error: outputRes.error }
        return result
    }

    var outputData = outputRes.value
    if (outputData.rawOutput) {
        outputEl.textContent = outputData.rawOutput
    } else {
        outputEl.textContent = ""
    }

    // --- Populate summary UI ---
    var summary = outputData.summary
    if (!summary) {
        result = { ok: false, error: "fetchDeploymentOutput: no summary data" }
        return result
    }

    // Fill in top status block
    var statusTextEl = document.querySelector('[data-field="status-text"]')
    if (statusTextEl) {
        statusTextEl.textContent = summary.Status
    }

    var startTimeEl = document.querySelector('[data-field="start-time"]')
    if (startTimeEl) {
        startTimeEl.textContent = summary["Start-Time"]
    }

    var endTimeEl = document.querySelector('[data-field="end-time"]')
    if (endTimeEl) {
        endTimeEl.textContent = summary["End-Time"]
    }

    var elapsedTimeEl = document.querySelector('[data-field="elapsed-time"]')
    if (elapsedTimeEl) {
        elapsedTimeEl.textContent = summary["Elapsed-Time"]
    }

    var transferredSizeEl = document.querySelector('[data-field="transferred-size"]')
    if (transferredSizeEl) {
        transferredSizeEl.textContent = summary["Transferred-Size"]
    }

    var commitIdEl = document.querySelector('[data-field="commit-id"]')
    if (commitIdEl) {
        commitIdEl.textContent = summary["Deployment-Commit-Hash"]
    }

    // Counters
    var hostsTotalEl = document.querySelector('[data-field="hosts-total"]')
    if (hostsTotalEl) {
        hostsTotalEl.textContent = summary.Counters.Hosts.toString()
    }

    var hostsCompletedEl = document.querySelector('[data-field="hosts-completed"]')
    if (hostsCompletedEl) {
        hostsCompletedEl.textContent = summary.Counters["Hosts-Completed"].toString()
    }

    var hostsFailedEl = document.querySelector('[data-field="hosts-failed"]')
    if (hostsFailedEl) {
        hostsFailedEl.textContent = summary.Counters["Hosts-Failed"].toString()
    }

    var itemsTotalEl = document.querySelector('[data-field="items-total"]')
    if (itemsTotalEl) {
        itemsTotalEl.textContent = summary.Counters.Items.toString()
    }

    var itemsCompletedEl = document.querySelector('[data-field="items-completed"]')
    if (itemsCompletedEl) {
        itemsCompletedEl.textContent = summary.Counters["Items-Completed"].toString()
    }

    var itemsFailedEl = document.querySelector('[data-field="items-failed"]')
    if (itemsFailedEl) {
        itemsFailedEl.textContent = summary.Counters["Items-Failed"].toString()
    }

    // Highlight status visually
    var statusBox = getElement("summary-status")
    statusBox.classList.remove("success", "error")
    if (summary.Status.toLowerCase() === "deployed" || summary.Status.toLowerCase() === "success") {
        statusBox.classList.add("success")
    } else {
        statusBox.classList.add("error")
    }

    // Fill in host details
    var hostListEl = getElement("host-list")
    hostListEl.innerHTML = ""

    if (summary.Hosts && summary.Hosts.length > 0) {
        for (var hostIndex = 0; hostIndex < summary.Hosts.length; hostIndex++) {
            const host = summary.Hosts[hostIndex]
            if (host == null) continue;
            var detailsEl = document.createElement("details")
            detailsEl.className = "host-summary"

            var statusIcon = "✗"
            var statusClass = "Failed"
            if (host.Status === "Deployed") {
                statusIcon = "✓"
                statusClass = "Deployed"
            }

            var hostMetaHtml = "<div class=\"host-meta\">"
            if (host["Error-Message"]) {
                hostMetaHtml += `<div><strong>Error:</strong> ${escapeHtml(String(host["Error-Message"]))}</div>`
            }
            if (host["Total-Items"]) {
                hostMetaHtml += `<div><strong>Items:</strong> ${escapeHtml(String(host["Total-Items"]))}</div>`
            }
            if (host["Transferred-Size"]) {
                hostMetaHtml += `<div><strong>Transferred:</strong> ${escapeHtml(String(host["Transferred-Size"]))}</div>`
            }
            hostMetaHtml += "</div>"

            var itemsTableHtml = ""
            if (host.Items && host.Items.length > 0) {
                itemsTableHtml = "<table class=\"host-items\"><tbody>"
                for (var itemIndex = 0; itemIndex < host.Items.length; itemIndex++) {
                    const item = host.Items[itemIndex]
                    if (item == null) continue;

                    var itemStatusText = ""
                    var itemStatusClass = ""
                    if (item.Status === "Deployed") {
                        itemStatusText = "✓ Deployed"
                        itemStatusClass = "status success"
                    } else {
                        itemStatusText = "✗ Failed"
                        itemStatusClass = "status error"
                    }
                    var itemAction = item["Deployment-Action"] != null ? escapeHtml(String(item["Deployment-Action"])) : ""

                    itemsTableHtml += "<tr>"
                    itemsTableHtml += "<td><span class=\"" + itemStatusClass + "\">" + itemStatusText + "</span></td>"
                    itemsTableHtml += "<td class=\"item-action\">" + itemAction + "</td>"
                    itemsTableHtml += "<td class=\"item-name\"><a href=\"/file.html?path=" + encodeURIComponent(item.Name) + "\">" + escapeHtml(item.Name) + "</a></td>"
                    itemsTableHtml += "</tr>"
                }
                itemsTableHtml += "</tbody></table>"
            }

            detailsEl.innerHTML = "<summary>" +
                '<span class="status ' + statusClass + '">' + statusIcon + '</span> ' +
                escapeHtml(host.Name) +
                '</summary> ' +
                hostMetaHtml + itemsTableHtml
            hostListEl.appendChild(detailsEl)
        }
    }

    // Unhide the summary panel
    summaryEl.classList.remove("hidden")
    result = { ok: true, value: undefined }
    return result
}

// Reset host override dropdown state on bfcache restore
window.addEventListener("pageshow", (event) => {
    if (event.persisted) {
        resetHostsDropdownState()
        var inputEl = document.getElementById("overridehosts-input")
        if (inputEl instanceof HTMLInputElement) {
            inputEl.value = ""
        }
    }
})

window.addEventListener("DOMContentLoaded", () => {
    initPage()

    const params = new URLSearchParams(window.location.search)

    const commitID = params.get("commitid")
    if (commitID) {
        const inputEl = document.getElementById("commitid")
        if (inputEl instanceof HTMLInputElement) {
            inputEl.value = commitID
        }
    }

    const autorollbackParam = params.get("autorollback")
    if (autorollbackParam === "true") {
        const checkbox = document.getElementById("opt-autorollback")
        if (checkbox instanceof HTMLInputElement) {
            checkbox.checked = true
        }
    }

    setupOverrideHostsDropdown()

    const deployFinalBtn = getElement("deploy-btn") as HTMLButtonElement | null
    if (deployFinalBtn) {
        deployFinalBtn.addEventListener("click", handleDeployClick)
    }
})
