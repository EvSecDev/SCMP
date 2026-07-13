import { isErr, id } from "./lib/result.js"
import { getElement, mustElement, mustQuerySelector } from "./lib/dom/lookup.js"
import { createStatusSpan } from "./lib/dom/widgets.js"
import { getJSONViaJSON } from "./lib/rpc/client.js"
import { logError, logWarning } from "./lib/logging/log.js"
import { initPage } from "./lib/init/page.js"
import type { RepoStatus, RepoFileStatus, RepoCommitInfo, DiffFile, RepoFileDiffResp } from "./types/repository.js"

let latestStatus: RepoStatus | null = null

/* ---------- UI Entry Point ---------- */
let repoUIInitialized = false

async function initRepoUI() {
    await refreshStatus()
    await refreshHistory()
    hookCommitBox()
}

async function initRepoUIOnce() {
    initPage()

    if (repoUIInitialized) {
        return
    }
    repoUIInitialized = true
    initPaginationElements()
    await initRepoUI()
}

// Run on back/forward navigation if page was persisted (bfcache) - registered
// before variable declarations to avoid TDZ; variables are hoisted as `undefined`.
window.addEventListener("pageshow", (event) => {
    if (event.persisted) {
        repoUIInitialized = false;
        stageHooksInitialized = false;
        refreshStatus();
    }
});

// Run on DOM ready
document.addEventListener("DOMContentLoaded", initRepoUIOnce);

/* ---------- Status Panel ---------- */
async function refreshStatus() {
    const result = await getJSONViaJSON<RepoStatus>("repo.staging.status")
    if (isErr(result)) {
        logError(`refreshStatus: ${result.error}`, false)
        return
    }

    latestStatus = result.value

    var lists = document.querySelectorAll(".git-panel .git-file-list")
    if (lists.length >= 2) {
        const list0 = lists[0];
        const list1 = lists[1];
        if (list0 == null || list1 == null) {
            return
        }
        renderFileList(list0, result.value.unstaged, "unstaged")
        renderFileList(list1, result.value.staged, "staged")
    }

    hookStageUnstageAll()
}

function renderFileList(
    container: Element,
    files: RepoFileStatus[],
    type: "staged" | "unstaged"
) {
    container.innerHTML = ""
    for (var fileIndex = 0; fileIndex < files.length; fileIndex++) {
        const file = files[fileIndex];
        if (file == null) continue;
        var li = document.createElement("li")

        var btn = document.createElement("button")
        if (type === "unstaged") {
            btn.className = "btn small btn-stage"
            btn.textContent = "Stage"
        } else {
            btn.className = "btn small btn-unstage"
            btn.textContent = "Unstage"
        }
        btn.onclick = () => toggleStage([file.path], type)

        var link = document.createElement("a")
        link.className = "file-name"
        link.textContent = file.path
        link.href = `/file.html?path=${encodeURIComponent(file.path)}`

        var statusSpan = createStatusSpan(file.status)

        li.appendChild(btn)
        li.appendChild(link)
        li.appendChild(statusSpan)
        container.appendChild(li)
    }
}

async function toggleStage(paths: string[], type: "staged" | "unstaged") {
    var method: string
    if (type === "unstaged") {
        method = "repo.staging.add"
    } else {
        method = "repo.staging.remove"
    }
    const payload = { paths: paths }

    const result = await getJSONViaJSON(method, payload)
    if (isErr(result)) {
        var action: string
        if (type === "unstaged") {
            action = "stage"
        } else {
            action = "unstage"
        }
        logError(`toggleStage: ${action}: ${result.error}`, false)
        return
    }

    refreshFileList(result.value)
}

function refreshFileList(newStatus: RepoStatus) {
    latestStatus = newStatus
    var lists = document.querySelectorAll(".git-panel .git-file-list")
    if (lists.length >= 2) {
        const list0 = lists[0];
        const list1 = lists[1];
        if (list0 == null || list1 == null) {
            return
        }
        renderFileList(list0, newStatus.unstaged, "unstaged")
        renderFileList(list1, newStatus.staged, "staged")
    }

    hookStageUnstageAll()
}

/* ---------- Stage/Unstage All ---------- */
let stageAllBtn: HTMLButtonElement | null = null
let stageRefreshBtn: HTMLButtonElement | null = null
let unstageAllBtn: HTMLButtonElement | null = null
let stageHooksInitialized = false

function hookStageUnstageAll() {
    if (stageHooksInitialized) {
        return
    }
    stageHooksInitialized = true

    var stageAllResult = mustQuerySelector<HTMLButtonElement>(".git-panel .btn-stage")
    if (isErr(stageAllResult)) {
        logWarning(`hookStageUnstageAll: ${stageAllResult.error}`)
        return
    }
    stageAllBtn = stageAllResult.value

    var stageRefreshResult = mustQuerySelector<HTMLButtonElement>(".git-panel .btn-refresh")
    if (isErr(stageRefreshResult)) {
        logWarning(`hookStageUnstageAll: ${stageRefreshResult.error}`)
        return
    }
    stageRefreshBtn = stageRefreshResult.value

    var unstageAllResult = mustQuerySelector<HTMLButtonElement>(".git-panel .btn-unstage")
    if (isErr(unstageAllResult)) {
        logWarning(`hookStageUnstageAll: ${unstageAllResult.error}`)
        return
    }
    unstageAllBtn = unstageAllResult.value

    var updateButtonState = () => {
        if (!latestStatus) {
            return
        }
        if (stageAllBtn) {
            stageAllBtn.disabled = latestStatus.unstaged.length === 0
        }
        if (unstageAllBtn) {
            unstageAllBtn.disabled = latestStatus.staged.length === 0
        }
    }

    if (stageRefreshBtn) {
        stageRefreshBtn.onclick = async () => {
            const result = await getJSONViaJSON("repo.artifacts.refresh")
            if (isErr(result)) {
                logError(`hookStageUnstageAll: refresh artifacts: ${result.error}`, false)
                return
            }

            latestStatus = result.value
            updateButtonState()
            refreshFileList(result.value)
        }
    }

    if (stageAllBtn) {
        stageAllBtn.onclick = async () => {
            if (!latestStatus) {
                return
            }
            var paths: string[] = []
            for (var fileIndex = 0; fileIndex < latestStatus.unstaged.length; fileIndex++) {
                const f = latestStatus.unstaged[fileIndex];
                if (f == null) continue;
                paths.push(f.path)
            }
            if (paths.length === 0) {
                return
            }

            await toggleStage(paths, "unstaged")
            updateButtonState()
        }
    }

    if (unstageAllBtn) {
        unstageAllBtn.onclick = async () => {
            if (!latestStatus) {
                return
            }
            var paths: string[] = []
            for (var fileIndex = 0; fileIndex < latestStatus.staged.length; fileIndex++) {
                const f = latestStatus.staged[fileIndex];
                if (f == null) continue;
                paths.push(f.path)
            }
            if (paths.length === 0) {
                return
            }

            await toggleStage(paths, "staged")
            updateButtonState()
        }
    }

    updateButtonState()
}

/* ---------- Commit Box ---------- */
function hookCommitBox() {
    var textareaResult = mustElement<HTMLTextAreaElement>(id("commit-message"))
    if (isErr(textareaResult)) {
        logWarning(`hookCommitBox: ${textareaResult.error}`)
        return
    }
    var textarea = textareaResult.value

    var commitResult = mustElement<HTMLButtonElement>(id("commit-button"))
    if (isErr(commitResult)) {
        logWarning(`hookCommitBox: ${commitResult.error}`)
        return
    }
    var commitBtn = commitResult.value

    var commitDeployResult = mustElement<HTMLButtonElement>(id("commit-deploy-button"))
    if (isErr(commitDeployResult)) {
        logWarning(`hookCommitBox: ${commitDeployResult.error}`)
        return
    }
    var commitDeployBtn = commitDeployResult.value

    var updateButtonState = () => {
        var isEmpty = textarea.value.trim().length === 0
        commitBtn.disabled = isEmpty
        commitDeployBtn.disabled = isEmpty
    }

    textarea.addEventListener("input", updateButtonState)
    updateButtonState()

    var handleCommit = async (redirectToDeploy: boolean) => {
        var message = textarea.value.trim()
        if (!message) {
            return
        }

        var result = await getJSONViaJSON<any, RepoCommitInfo>("repo.commit", { message: message })
        if (isErr(result)) {
            logError(`handleCommit: ${result.error}`, false)
            return
        }

        var commitid = result.value.fullHash

        if (redirectToDeploy) {
            var params = new URLSearchParams({
                commitid: commitid,
                autorollback: "true"
            })
            var deployLink = `/deployments.html?${params.toString()}`
            window.location.href = deployLink
            return
        }

        textarea.value = ""
        updateButtonState()

        await refreshStatus()
        await refreshHistory()
    }

    commitBtn.addEventListener("click", () => handleCommit(false))
    commitDeployBtn.addEventListener("click", () => handleCommit(true))
}

/* ---------- History Table ---------- */
var currentOffset = 0
var currentLimit = 10
var rowsSelectEl: HTMLSelectElement = document.createElement("select")
var prevBtn: HTMLButtonElement = document.createElement("button")
var nextBtn: HTMLButtonElement = document.createElement("button")

function initPaginationElements() {
    var rowsSelect = getElement("rows-select")
    if (rowsSelect instanceof HTMLSelectElement) {
        rowsSelectEl = rowsSelect
    }
    rowsSelectEl.addEventListener("change", () => {
        currentLimit = parseInt(rowsSelectEl.value, 10)
        currentOffset = 0
        refreshHistory(currentOffset, currentLimit)
    })

    var prevBtnResult = mustQuerySelector<HTMLButtonElement>(".pagination button:first-child")
    if (prevBtnResult.ok) {
        prevBtn = prevBtnResult.value
    }

    var nextBtnResult = mustQuerySelector<HTMLButtonElement>(".pagination button:last-child")
    if (nextBtnResult.ok) {
        nextBtn = nextBtnResult.value
    }

    prevBtn.addEventListener("click", () => {
        currentOffset = Math.max(0, currentOffset - currentLimit)
        refreshHistory(currentOffset, currentLimit)
    })

    nextBtn.addEventListener("click", () => {
        currentOffset += currentLimit
        refreshHistory(currentOffset, currentLimit)
    })
}

async function refreshHistory(reqOffset?: number, reqLimit?: number) {
    var offset = reqOffset != null ? reqOffset : currentOffset
    var limit = reqLimit != null ? reqLimit : currentLimit
    currentOffset = offset
    currentLimit = limit

    const result = await getJSONViaJSON<any, RepoCommitInfo[]>("repo.commit.history", { limit: limit, offset: offset })
    if (isErr(result)) {
        logError(`refreshHistory: ${result.error}`, false)
        return
    }

    var commits = result.value
    var tbodyResult = mustQuerySelector<HTMLTableSectionElement>("#commit-history-table tbody")
    if (isErr(tbodyResult)) {
        logError(`refreshHistory: ${tbodyResult.error}`, false)
        return
    }
    var tbody = tbodyResult.value
    tbody.innerHTML = ""

    prevBtn.disabled = offset === 0
    nextBtn.disabled = commits.length < limit

    var pageInfoResult = mustQuerySelector<HTMLSpanElement>(".pagination .page-info")
    if (isErr(pageInfoResult)) {
        return
    }
    pageInfoResult.value.textContent = `Page ${Math.floor(offset / limit) + 1}`

    for (var commitIndex = 0; commitIndex < commits.length; commitIndex++) {
        const commit = commits[commitIndex];
        if (commit == null) continue;
        let prevCommitHash: string
        if (commitIndex < commits.length - 1) {
            const nextCommit = commits[commitIndex + 1];
            if (nextCommit) {
                prevCommitHash = nextCommit.fullHash
            } else {
                prevCommitHash = ""
            }
        } else {
            prevCommitHash = ""
        }

        var tr = document.createElement("tr")
        const td1 = document.createElement("td")
        td1.textContent = commit.shortHash
        tr.appendChild(td1)

        const td2 = document.createElement("td")
        td2.textContent = commit.date
        tr.appendChild(td2)

        const td3 = document.createElement("td")
        td3.textContent = commit.authorName + " (" + commit.authorEmail + ")"
        tr.appendChild(td3)

        const td4 = document.createElement("td")
        td4.textContent = commit.numberOfChanges.toString()
        tr.appendChild(td4)

        const td5 = document.createElement("td")
        td5.textContent = commit.message
        tr.appendChild(td5)

        const td6 = document.createElement("td")
        const showBtnEl = document.createElement("button")
        showBtnEl.className = "btn btn-show-files"
        showBtnEl.textContent = "Show"
        const deployBtnEl = document.createElement("button")
        deployBtnEl.className = "btn btn-deploy"
        deployBtnEl.textContent = "Deploy"
        td6.appendChild(showBtnEl)
        td6.appendChild(deployBtnEl)
        tr.appendChild(td6)
        tbody.appendChild(tr)

        const fileTr = document.createElement("tr")
        fileTr.classList.add("commit-file-row", "hidden")
        const fileTd = document.createElement("td")
        fileTd.setAttribute("colspan", "6")
        const fileListDiv = document.createElement("div")
        fileListDiv.className = "expanded-file-list"

        for (var fileIndex = 0; fileIndex < commit.filesChanged.length; fileIndex++) {
            const fileInfo = commit.filesChanged[fileIndex];
            if (fileInfo == null) continue;
            var fileRow = document.createElement("div")
            fileRow.className = "file-row"

            var pathSpan = document.createElement("span")
            pathSpan.textContent = fileInfo.path
            fileRow.appendChild(pathSpan)

            var statusContainer = document.createElement("div")
            statusContainer.appendChild(createStatusSpan(fileInfo.status))
            fileRow.appendChild(statusContainer)

            var actions = document.createElement("div")
            actions.className = "file-actions"

            var editBtn = document.createElement("button")
            editBtn.textContent = "Edit"
            editBtn.className = "btn"
            editBtn.onclick = () => {
                window.location.href = `/file.html?path=${encodeURIComponent(fileInfo.path)}`
            }
            if (fileInfo.status === "deleted") {
                editBtn.disabled = true
                editBtn.classList.add("disabled")
            }
            actions.appendChild(editBtn)

            var diffBtn = document.createElement("button")
            diffBtn.textContent = "Diff"
            diffBtn.className = "btn"
            diffBtn.onclick = async () => {
                var baseHash = prevCommitHash

                if (!baseHash) {
                    var queryOffset = offset + limit + 1
                    var res = await getJSONViaJSON<any, RepoCommitInfo[]>("repo.commit.history", { limit: 1, offset: queryOffset })
                    if (!isErr(res) && res.value.length > 0) {
                        const first = res.value[0]
                        if (first) {
                            baseHash = first.fullHash
                        } else {
                            baseHash = ""
                        }
                    } else {
                        logWarning("refreshHistory: no base commit found")
                        baseHash = ""
                    }
                }

                var payload = {
                    path: fileInfo.path,
                    targetCommit: commit.fullHash,
                    baseCommit: baseHash
                }

                var diffResult = await getJSONViaJSON<any, RepoFileDiffResp>("repo.commit.diff", payload)
                if (isErr(diffResult)) {
                    logError(`refreshHistory: fetch diff: ${diffResult.error}`, false)
                    return
                }

                showDiffModal(diffResult.value.files)
            }
            actions.appendChild(diffBtn)

            fileRow.appendChild(actions)
            fileListDiv.appendChild(fileRow)
        }

        fileTd.appendChild(fileListDiv)
        fileTr.appendChild(fileTd)
        tbody.appendChild(fileTr)

        const showBtn = tr.querySelector<HTMLButtonElement>(".btn-show-files")
        if (showBtn) {
            const btn = showBtn
            showBtn.onclick = () => {
                if (fileTr.classList.contains("hidden")) {
                    fileTr.classList.remove("hidden")
                    btn.textContent = "Hide"
                } else {
                    fileTr.classList.add("hidden")
                    btn.textContent = "Show"
                }
            }
        }

        var deployBtn = tr.querySelector<HTMLButtonElement>(".btn-deploy")
        if (deployBtn) {
            deployBtn.onclick = () => {
                window.location.href = `/deployments.html?commitid=${encodeURIComponent(commit.fullHash)}`
            }
        }
    }
}

function showDiffModal(diffInfo: DiffFile[]) {
    var diffText = ""

    for (var fileIndex = 0; fileIndex < diffInfo.length; fileIndex++) {
        var file = diffInfo[fileIndex]
        if (file == null) continue;
        var oldPath = "/dev/null"
        if (file.old_path) {
            oldPath = file.old_path
        }
        var newPath = "/dev/null"
        if (file.new_path) {
            newPath = file.new_path
        }

        var fileHeader = `--- ${oldPath}\n+++ ${newPath}\n`
        diffText += fileHeader

        if (file.is_binary) {
            var binaryPath = file.old_path
            if (file.new_path) {
                binaryPath = file.new_path
            }
            diffText += `Binary file changed: ${binaryPath}\n\n`
            continue
        }

        if (!file.hunks) {
            continue
        }

        for (var hunkIndex = 0; hunkIndex < file.hunks.length; hunkIndex++) {
            var hunk = file.hunks[hunkIndex]
            if (hunk == null) continue;
            var changes = hunk.changes
            var oldStartLine = hunk.old_start_line
            var newStartLine = hunk.new_start_line
            var oldLineCount = hunk.old_line_count
            var newLineCount = hunk.new_line_count

            // Find change indexes
            var changeIndexes: number[] = []
            for (var changeIndex = 0; changeIndex < changes.length; changeIndex++) {
                const c = changes[changeIndex];
                if (c == null) continue;
                if (c.type !== "context") {
                    changeIndexes.push(changeIndex)
                }
            }

            var printedIndexes = new Set<number>()
            var hunkLines: string[] = []

            // Emit hunk header once per hunk
            hunkLines.push(`@@ -${oldStartLine},${oldLineCount} +${newStartLine},${newLineCount} @@`)

            for (var changeIdx = 0; changeIdx < changeIndexes.length; changeIdx++) {
                const idx = changeIndexes[changeIdx]
                if (idx == null) continue
                var start = Math.max(0, idx - 5)
                var end = changes.length
                var calcEnd = idx + 6
                if (calcEnd < end) {
                    end = calcEnd
                }

                var alreadyPrinted = false
                for (var k = start; k < end; k++) {
                    if (printedIndexes.has(k)) {
                        alreadyPrinted = true
                        break
                    }
                }
                if (alreadyPrinted) {
                    continue
                }

                for (var k = start; k < end; k++) {
                    printedIndexes.add(k)
                }

                for (var k = start; k < end; k++) {
                    var change = changes[k]
                    if (change == null) continue;
                    var prefix = " "
                    if (change.type === "add") {
                        prefix = "+"
                    } else if (change.type === "del") {
                        prefix = "-"
                    }
                    hunkLines.push(prefix + change.content)
                }

                hunkLines.push("")
            }

            diffText += `${hunkLines.join("\n")}\n`
        }
    }

    var overlay = document.createElement("div")
    Object.assign(overlay.style, {
        position: "fixed",
        top: "0",
        left: "0",
        width: "100vw",
        height: "100vh",
        background: "rgba(0,0,0,0.5)",
        display: "flex",
        justifyContent: "center",
        alignItems: "center",
        zIndex: "1000",
        padding: "1rem",
        boxSizing: "border-box",
    })

    var modalEl = document.createElement("div")
    Object.assign(modalEl.style, {
        backgroundColor: "#1f1f1f",
        color: "#fff",
        padding: "1.5rem",
        borderRadius: "0.75rem",
        maxWidth: "90vw",
        maxHeight: "80vh",
        overflow: "auto",
        display: "flex",
        flexDirection: "column",
        gap: "1rem",
        textAlign: "left",
        fontFamily: "monospace",
        whiteSpace: "pre",
    })

    var textContainer = document.createElement("div")
    textContainer.textContent = diffText

    var buttonsEl = document.createElement("div")
    Object.assign(buttonsEl.style, {
        display: "flex",
        justifyContent: "center",
        marginTop: "1rem",
    })

    var backBtn = document.createElement("button")
    backBtn.textContent = "Back"
    Object.assign(backBtn.style, {
        padding: "0.3rem 1.2rem",
        fontSize: "1rem",
        cursor: "pointer",
        backgroundColor: "#333",
        color: "#ddd",
        border: "none",
        borderRadius: "0.25rem",
        transition: "background 0.3s",
    })

    backBtn.onmouseenter = () => { backBtn.style.backgroundColor = "#444" }
    backBtn.onmouseleave = () => { backBtn.style.backgroundColor = "#333" }
    backBtn.onclick = () => document.body.removeChild(overlay)

    buttonsEl.appendChild(backBtn)
    modalEl.appendChild(textContainer)
    modalEl.appendChild(buttonsEl)
    overlay.appendChild(modalEl)
    document.body.appendChild(overlay)
}
