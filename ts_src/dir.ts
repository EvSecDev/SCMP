import { isErr } from "./lib/result.js"
import type { Result } from "./lib/result.js"
import { getElement } from "./lib/dom/lookup.js"
import { safeCopyToClipboard } from "./lib/dom/clipboard.js"
import { getJSONViaJSON } from "./lib/rpc/client.js"
import { logError, logWarning, logAlert } from "./lib/logging/log.js"
import { initPage } from "./lib/init/page.js"
import { createPagination } from "./lib/dom/widgets.js"
import { showModal } from "./ui/modal.js"
import type { FileMetadata, FileOp, FileMove, FilePathSearchReq, FilePathSearchResults } from "./types/filesystem.js"

// State
var currentPath = "/"
var allFiles: FileMetadata[] = []

// DOM references (initialized in initDirUI)
var tableBody: HTMLTableSectionElement = document.createElement("tbody")
var paginationInfo: HTMLSpanElement = document.createElement("span")
var pathHeader: HTMLElement = document.createElement("div")
var createBtn: HTMLElement = document.createElement("button")
var searchInput: HTMLInputElement = document.createElement("input")
var searchBtn: HTMLButtonElement = document.createElement("button")
var clearSearchBtn: HTMLButtonElement = document.createElement("button")
var searchCog: HTMLElement = document.createElement("button")
var rowSelect: HTMLSelectElement = document.createElement("select")
var prevButton: HTMLButtonElement = document.createElement("button")
var nextButton: HTMLButtonElement = document.createElement("button")

var dirPagination: ReturnType<typeof createPagination> | null = null

function initDirUI() {
    var tableBodyResult = getElement("file-tbody") as HTMLTableSectionElement | null
    if (!tableBodyResult) {
        logError(`initDirUI: missing file-tbody`, true)
        return
    }
    tableBody = tableBodyResult

    var paginationInfoResult = getElement("pagination-info") as HTMLSpanElement | null
    if (!paginationInfoResult) {
        logError(`initDirUI: missing pagination-info`, true)
        return
    }
    paginationInfo = paginationInfoResult

    prevButton = getElement("pagination-prev") as HTMLButtonElement
    nextButton = getElement("pagination-next") as HTMLButtonElement
    if (!prevButton || !nextButton) {
        logError(`initDirUI: missing pagination buttons`, true)
        return
    }

    pathHeader = getElement("current-path")
    createBtn = getElement("create-btn")
    searchInput = getElement("search-input") as HTMLInputElement
    searchBtn = getElement("search-btn") as HTMLButtonElement
    clearSearchBtn = getElement("clear-search-btn") as HTMLButtonElement
    searchCog = getElement("search-cog")
    rowSelect = getElement("rows-select") as HTMLSelectElement

    dirPagination = createPagination(
        () => allFiles.length,
        rowSelect,
        prevButton,
        nextButton,
        paginationInfo,
        () => { renderPage() }
    )

    // Wire up event listeners
    createBtn.addEventListener("click", () => {
        showModal({
            message: "Enter name for new file or directory:",
            inputPlaceholder: "Name",
            confirmText: "Create",
            cancelText: "Cancel",
            selects: [
                {
                    id: "create-type-select",
                    label: "Type",
                    options: ["file", "directory"],
                    default: "file"
                }
            ],
            onConfirm: async (inputValue, _checkboxValues, selectValues) => {
                if (!inputValue) {
                    return
                }

                var type = "file" as "file" | "directory"
                if (selectValues && selectValues["create-type-select"]) {
                    type = selectValues["create-type-select"] as "file" | "directory"
                }

                var nPath = normalizePath(`${currentPath}/${inputValue}`)
                var res = await getJSONViaJSON("fs.item.new", {
                    path: nPath,
                    type: type,
                    recursive: false,
                })
                if (isErr(res)) {
                    logError(`createEntry: ${type} ${nPath}: ${res.error}`, false)
                } else {
                    await refreshDirectoryListing()
                }
            }
        })
    })

    var refreshBtn = getElement("refresh-btn") as HTMLButtonElement
    refreshBtn.addEventListener("click", async () => {
        await refreshDirectoryListing()
    })

    searchInput.addEventListener("keydown", (e) => {
        if (e.key === "Enter") {
            e.preventDefault()
            performSearch(searchInput.value)
        }
    })

    searchBtn.addEventListener("click", () => {
        performSearch(searchInput.value)
    })

    clearSearchBtn.addEventListener("click", async () => {
        await refreshDirectoryListing()
        searchInput.value = ""
    })

    searchCog.addEventListener("click", () => {
        showModal({
            message: "Search options",
            confirmText: "Save",
            cancelText: "Cancel",
            selects: [
                {
                    id: "select-query-type",
                    label: "Match Type",
                    options: ["contains", "exact", "prefix", "suffix"],
                    default: searchOptions.matchType,
                },
                {
                    id: "select-file-type",
                    label: "File Type",
                    options: ["all", "file", "directory"],
                    default: searchOptions.fileType,
                }
            ],
            inputs: [
                {
                    id: "input-depth",
                    label: "Depth of search",
                    placeholder: "10",
                    default: searchOptions.depth.toString(),
                }
            ],
            onConfirm: (_inputValue, _checkboxValues, selectValues, inputValues) => {
                var matchType = "contains"
                if (selectValues && selectValues["select-query-type"]) {
                    matchType = selectValues["select-query-type"]
                }
                searchOptions.matchType = matchType

                var fileType = "all"
                if (selectValues && selectValues["select-file-type"]) {
                    fileType = selectValues["select-file-type"]
                }
                searchOptions.fileType = fileType

                var depthStr = "10"
                if (inputValues && inputValues["input-depth"]) {
                    depthStr = inputValues["input-depth"]
                }
                var parsedDepth = parseInt(depthStr, 10)
                if (Number.isNaN(parsedDepth) || parsedDepth < 0) {
                    logAlert("Depth must be a non-negative number.")
                    return false
                }
                searchOptions.depth = parsedDepth
                return true
            }
        })
    })
}

// Helpers
function normalizePath(path: string): string {
    if (!path.startsWith("/")) {
        path = `/${path}`
    }
    return path.replace(/\/+/g, "/")
}

// Delete a file or directory
async function deleteEntry(entry: FileOp): Promise<Result<void>> {
    const op: FileOp = {
        path: entry.path,
        type: entry.type,
        recursive: entry.type === "directory",
    }
    const res = await getJSONViaJSON("fs.item.delete", op)
    if (isErr(res)) {
        logError(`deleteEntry: ${entry.path}: ${res.error}`, false)
    }
    return res
}

// Fetch directory listing using helper
async function fetchFiles(req: { path: string }): Promise<FileMetadata[]> {
    const res: Result<FileMetadata[]> = await getJSONViaJSON("fs.directory.list", { path: req.path })
    if (isErr(res)) {
        logError(`fetchFiles: ${req.path}: ${res.error}`, false)
        return []
    }

    var entries: FileMetadata[] = []
    if (Array.isArray(res.value)) {
        entries = res.value
    }

    if (entries.length === 0) {
        logWarning(`fetchFiles: directory ${req.path} is empty`)
    }

    return entries
}

async function refreshDirectoryListing(): Promise<void> {
    allFiles = await fetchFiles({ path: currentPath })
    isSearchResults = false
    if (dirPagination) {
        dirPagination.setPage(1)
    }
    renderPage()
}

async function searchFiles(req: FilePathSearchReq): Promise<FilePathSearchResults> {
    const res: Result<FilePathSearchResults> = await getJSONViaJSON("fs.item.search", req)
    if (isErr(res)) {
        logError(`searchFiles: ${req.query} in ${req.path}: ${res.error}`, false)
        return {
            orig: req,
            matchCount: 0,
            matches: [],
        }
    }

    return res.value
}

// Move a file or directory
export async function moveEntry(move: FileMove): Promise<Result<void>> {
    const res = await getJSONViaJSON("fs.item.move", move)
    if (isErr(res)) {
        logError(`moveEntry: ${move.sourcePath} to ${move.destinationPath}: ${res.error}`, false)
    }
    return res
}

// Load directory
async function loadDirectory(path: string): Promise<void> {
    currentPath = normalizePath(path)
    await refreshDirectoryListing()
}

interface SearchOptions {
    matchType: string
    fileType: string
    depth: number
}
var searchOptions: SearchOptions = {
    matchType: "contains",
    fileType: "all",
    depth: 10,
}
var isSearchResults = false

async function performSearch(query: string) {
    if (!query.trim()) {
        return
    }

    var results = await searchFiles({
        path: currentPath,
        query: query,
        searchType: searchOptions.matchType,
        fileType: searchOptions.fileType,
        depth: searchOptions.depth,
    })

    var matches: FileMetadata[] = []
    if (results.matches && Array.isArray(results.matches)) {
        matches = results.matches
    }
    allFiles = matches

    if (allFiles.length === 0) {
        logWarning(`performSearch: no results for ${query}`)
    }
    isSearchResults = true
    if (dirPagination) {
        dirPagination.setPage(1)
    }
    renderPage()
}

// Render table page
function renderPage(): void {
    tableBody.innerHTML = ""
    var currentPage = 1
    var pageSize = 10
    if (dirPagination) {
        currentPage = dirPagination.getPage()
        pageSize = dirPagination.getPageSize()
    }

    var pageFiles = allFiles.slice((currentPage - 1) * pageSize, currentPage * pageSize)

    for (var pageFileIndex = 0; pageFileIndex < pageFiles.length; pageFileIndex++) {
        const pageFileItem = pageFiles[pageFileIndex];
        if (pageFileItem == null) {
            continue
        }
        const pageFile = pageFileItem;
        var row = document.createElement("tr")

        var nameCell = document.createElement("td")
        var displayName: string
        if (isSearchResults) {
            displayName = pageFile.path
        } else {
            var filePathParts = pageFile.path.split("/")
            var nonEmptyParts: string[] = []
            for (var filePathPartIndex = 0; filePathPartIndex < filePathParts.length; filePathPartIndex++) {
                const fp = filePathParts[filePathPartIndex]
                if (fp) {
                    nonEmptyParts.push(fp)
                }
            }
            var parts = nonEmptyParts
            if (parts.length > 0) {
                const lastPart = parts[parts.length - 1]
                if (lastPart) {
                    displayName = lastPart
                } else {
                    displayName = pageFile.path
                }
            } else {
                displayName = pageFile.path
            }
        }

        var link = document.createElement("span")
        link.textContent = displayName
        link.style.cursor = "pointer"
        link.classList.add("breadcrumb-segment")
        if (pageFile.type === "directory") {
            link.classList.add("directory")
        }

        link.addEventListener("click", async () => {
            if (pageFile.type === "directory") {
                await loadDirectory(normalizePath(pageFile.path))
            } else {
                window.location.href = `/file.html?path=${encodeURIComponent(normalizePath(pageFile.path))}`
            }
        })

        nameCell.appendChild(link)
        row.appendChild(nameCell)

        row.appendChild(createCell(pageFile.permissions))
        row.appendChild(createCell(pageFile.ownerName))
        row.appendChild(createCell(pageFile.groupName))

        var sizeText: string
        if (pageFile.size === 0 || pageFile.size == null) {
            sizeText = "-"
        } else {
            sizeText = pageFile.size.toString()
        }
        row.appendChild(createCell(sizeText))

        var modifiedText: string
        if (pageFile.lastModified == null) {
            modifiedText = "-"
        } else {
            modifiedText = pageFile.lastModified
        }
        row.appendChild(createCell(modifiedText))

        var actionsCell = document.createElement("td")
        actionsCell.classList.add("actions-cell")

        var moveBtn = document.createElement("button")
        moveBtn.textContent = "Move"
        moveBtn.classList.add("btn", "btn-move")
        moveBtn.onclick = () => {
            showModal({
                message: `Move/Copy '${pageFile.path}' to:`,
                inputPlaceholder: "Destination path",
                confirmText: "Submit",
                cancelText: "Cancel",
                checkboxes: [
                    { id: "deleteSource", label: "Delete source", default: false },
                    { id: "overwriteDestination", label: "Overwrite destination", default: false }
                ],
                onConfirm: async (inputValue, checkboxValues) => {
                    if (!inputValue) {
                        return
                    }

                    var deleteSource = false
                    var overwriteDestination = false
                    if (checkboxValues) {
                        if (checkboxValues.deleteSource) {
                            deleteSource = true
                        }
                        if (checkboxValues.overwriteDestination) {
                            overwriteDestination = true
                        }
                    }

                    await moveEntry({
                        sourcePath: pageFile.path,
                        destinationPath: inputValue,
                        deleteSource: deleteSource,
                        overwriteDestination: overwriteDestination
                    })

                    await refreshDirectoryListing()
                }
            })
        }

        var delBtn = document.createElement("button")
        delBtn.textContent = "Delete"
        delBtn.className = "btn"
        delBtn.onclick = () => {
            var pathParts = pageFile.path.split("/")
            var fileName = ""
            if (pathParts.length > 0) {
                const lastPart = pathParts[pathParts.length - 1]
                if (lastPart) {
                    fileName = lastPart
                }
            }

            var checkboxes: { id: string; label: string; default: boolean }[] = []
            if (pageFile.type === "directory") {
                checkboxes = [{ id: "recursive", label: "Recursive", default: false }]
            }

            showModal({
                message: `Confirm deletion of '${pageFile.path}':`,
                inputPlaceholder: `Type '${fileName}' to confirm`,
                checkboxes: checkboxes,
                confirmText: "Delete",
                cancelText: "Cancel",
                onConfirm: async (inputValue, checkboxValues) => {
                    if (inputValue !== fileName) {
                        logWarning("Name does not match. Deletion cancelled.")
                        return
                    }

                    var recursive = false
                    if (checkboxValues && checkboxValues["recursive"]) {
                        recursive = true
                    }

                    var res = await deleteEntry({
                        path: pageFile.path,
                        type: pageFile.type,
                        recursive: recursive,
                    })

                    if (isErr(res)) {
                        logError(`renderPage: delete ${pageFile.path}: ${res.error}`, false)
                    } else {
                        var filtered: FileMetadata[] = []
                        for (var fileIndex = 0; fileIndex < allFiles.length; fileIndex++) {
                            const af = allFiles[fileIndex]
                            if (af && af.path !== pageFile.path) {
                                filtered.push(af)
                            }
                        }
                        allFiles = filtered
                        renderPage()
                    }
                },
            })
        }

        actionsCell.appendChild(moveBtn)
        actionsCell.appendChild(delBtn)
        row.appendChild(actionsCell)

        tableBody.appendChild(row)
    }

    updatePathHeader()

    if (dirPagination) {
        dirPagination.refresh()
    }
}
function createCell(content: string): HTMLTableCellElement {
    const td = document.createElement("td")
    td.textContent = content
    return td
}

// Breadcrumb
function updatePathHeader() {
    pathHeader.innerHTML = ""
    var pathSegments = currentPath.split("/")
    var segments: string[] = []
    for (var segmentIndex = 0; segmentIndex < pathSegments.length; segmentIndex++) {
        const segItem = pathSegments[segmentIndex]
        if (segItem) {
            segments.push(segItem)
        }
    }

    var rootLink = document.createElement("span")
    rootLink.textContent = "/"
    rootLink.classList.add("breadcrumb-segment")
    rootLink.addEventListener("click", async () => {
        try {
            await loadDirectory("/")
        } catch (err) {
            logError(`updatePathHeader: loadDirectory '/': ${(err as Error).message}`, false)
        }
    })
    pathHeader.appendChild(rootLink)

    var builtPath = ""
    for (var segIndex = 0; segIndex < segments.length; segIndex++) {
        const segItem = segments[segIndex];
        if (segItem == null) {
            continue
        }
        const seg = segItem;
        const segPath = normalizePath(`${builtPath}/${seg}`);
        var sep = document.createElement("span")
        if (segIndex > 0) {
            sep.textContent = " / "
        } else {
            sep.textContent = " "
        }
        sep.classList.add("breadcrumb-separator")
        pathHeader.appendChild(sep)

        var segLink = document.createElement("span")
        segLink.textContent = seg
        segLink.classList.add("breadcrumb-segment")

        segLink.addEventListener("click", async () => {
            try {
                await loadDirectory(segPath)
            } catch (err) {
                logError(`updatePathHeader: loadDirectory '${segPath}': ${(err as Error).message}`, false)
            }
        })

        pathHeader.appendChild(segLink)

        builtPath += `/${seg}`
    }

    var copyBtn = document.createElement("button")
    copyBtn.textContent = "📋"
    copyBtn.classList.add("btn")
    copyBtn.title = "Copy full path"

    copyBtn.addEventListener("click", async (e) => {
        var copyResult = await safeCopyToClipboard(currentPath)
        if (isErr(copyResult)) {
            logError(`updatePath: copy path: ${copyResult.error}`, false)
            return
        }

        var check = document.createElement("span")
        check.textContent = "✔"
        check.style.position = "fixed"
        check.style.left = `${e.clientX}px`
        check.style.top = `${e.clientY}px`
        check.style.transform = "translate(-50%, -50%)"
        check.style.color = "#4caf50"
        check.style.fontSize = "1.6rem"
        check.style.pointerEvents = "none"
        check.style.opacity = "1"
        check.style.transition = "opacity 0.5s ease, transform 0.5s ease"
        document.body.appendChild(check)

        // Animate: float up and fade out
        requestAnimationFrame(() => {
            check.style.transform = "translate(-50%, -80%)"
            check.style.opacity = "0"
        })

        setTimeout(() => check.remove(), 1000)
    })
    pathHeader.appendChild(copyBtn)
}

// Init
window.addEventListener("DOMContentLoaded", () => {
    initDirUI()
    initPage()
    loadDirectory(currentPath)
})
