import { isErr } from "./lib/result.js"
import type { Result } from "./lib/result.js"
import { getElement } from "./lib/dom/lookup.js"
import { doFetch } from "./lib/http/fetch.js"
import { readText } from "./lib/http/body.js"
import { getJSONViaJSON, sendData } from "./lib/rpc/client.js"
import { logError, logWarning } from "./lib/logging/log.js"
import { initPage } from "./lib/init/page.js"
import type { FileMetadata, DownloadLink } from "./types/filesystem.js"

// --- DOM ELEMENTS (initialized after DOMReady) ---
var fileContainer: HTMLElement = document.createElement("div")
var filePathSpan: HTMLElement = document.createElement("span")
var contentEditor: HTMLTextAreaElement = document.createElement("textarea")
var contentToggleBtn: HTMLElement = document.createElement("button")
var contentCancelBtn: HTMLElement = document.createElement("button")
var editBtn: HTMLElement = document.createElement("button")
var cancelBtn: HTMLElement = document.createElement("button")

var isEditingContent = false
var isEditingMetadata = false
var originalVisibility: Map<HTMLDivElement, boolean> = new Map()

// --- Utility: Wait for DOM ---
function onDOMReady(): Promise<void> {
    return new Promise(resolve => {
        if (document.readyState === "loading") {
            document.addEventListener("DOMContentLoaded", () => resolve())
        } else {
            resolve()
        }
    })
}

function getFilePathFromQuery(): string | null {
    const params = new URLSearchParams(window.location.search)
    return params.get("path")
}

const metadataFieldMap: Record<string, string> = {
    ownerName: "metaOwnerVal",
    groupName: "metaGroupVal",
    permissions: "metaPermsVal",
    externalContentLocation: "metaExtCntLocVal",
    symbolicLinkTarget: "metaSymLinkTgtVal",
    dependencies: "metaDependenciesVal",
    preDeployCommands: "metaPreDeployCmdsVal",
    installCommands: "metaInstallCmdsVal",
    preApplyCommands: "metaPreApplyCmdsVal",
    postApplyCommands: "metaPostApplyCmdsVal",
    postInstallCommands: "metaPostInstallCmdsVal",
    reloadCommands: "metaReloadCmdsVal",
    reloadGroup: "metaReloadGroupVal",
}

interface FileDataModel {
    path: string
    contentLines: string[]
    metadata: FileMetadata | null
}

// Hold this files information here for source of truth
const fileData: FileDataModel = {
    path: "",
    contentLines: [],
    metadata: null,
}

/** Populate the file lines in the container */
function populateLines(container: HTMLElement, lines: string[]) {
    container.innerHTML = ""
    for (let i = 0; i < lines.length; i++) {
        const lineText = lines[i]
        if (lineText == null) continue;
        const lineDiv = document.createElement("div")
        if (lineText === "") {
            lineDiv.textContent = "\u00a0"
        } else {
            lineDiv.textContent = lineText
        }
        container.appendChild(lineDiv)
    }
}

/** Populate metadata fields */
function populateMetadata(metadata: FileMetadata | null) {
    var entries = Object.entries(metadataFieldMap)
    for (var entryIndex = 0; entryIndex < entries.length; entryIndex++) {
        const entry = entries[entryIndex];
        if (entry == null) continue;
        var key = entry[0]
        var valId = entry[1]
        var valueDiv = document.getElementById(valId)
        if (valueDiv == null) {
            continue
        }
        var container = valueDiv.parentElement
        if (container == null) {
            continue
        }

        var value: unknown = undefined
        if (metadata != null) {
            value = (metadata as Record<string, unknown>)[key]
        }

        if (value === undefined || value === null) {
            container.classList.add("hidden")
            continue
        }
        if (Array.isArray(value) && value.length === 0) {
            container.classList.add("hidden")
            continue
        }

        container.classList.remove("hidden")

        if (Array.isArray(value)) {
            valueDiv.innerHTML = ""
            for (var itemIndex = 0; itemIndex < value.length; itemIndex++) {
                var itemDiv = document.createElement("div")
                itemDiv.textContent = String(value[itemIndex])
                valueDiv.appendChild(itemDiv)
            }
        } else {
            valueDiv.textContent = String(value)
        }
    }
}

// --- Content Editor ---

function hookContentEditorEvents() {
    let isSaving = false

    contentToggleBtn.addEventListener("click", async () => {
        if (isSaving) {
            return
        }

        if (!isEditingContent) {
            contentEditor.value = fileData.contentLines.join("\n")
            fileContainer.classList.add("hidden")
            contentEditor.classList.remove("hidden")
            contentEditor.focus()
            resetEditorPosition(contentEditor)
            contentToggleBtn.textContent = "Save"
            contentToggleBtn.classList.remove("edit-mode")
            contentToggleBtn.classList.add("save-mode")
            isEditingContent = true
        } else {
            isSaving = true
            fileData.contentLines = contentEditor.value.split(/\r?\n/)
            const contentPayload = fileData.contentLines.join("\n")

            let path = fileData.path
            if (path == null) {
                path = ""
            }

            const res = await sendData("/data-store/upload", "POST", contentPayload, true)
            if (isErr(res)) {
                isSaving = false
                logError(`hookContentEditorEvents: upload: ${res.error}`, true)
                return
            }

            if (res.value === "" || res.value === undefined || res.value === null) {
                isSaving = false
                logError(`hookContentEditorEvents: no data ID for upload: path ${path}`, false)
                return
            }

            const saveRes = await getJSONViaJSON("fs.item.data.save", { path: path, dataID: res.value })
            if (isErr(saveRes)) {
                logError(`hookContentEditorEvents: save: ${saveRes.error}`, true)
                return
            }

            populateLines(fileContainer, fileData.contentLines)
            contentEditor.classList.add("hidden")
            fileContainer.classList.remove("hidden")
            resetEditorPosition(contentEditor)
            contentToggleBtn.textContent = "Edit"
            contentToggleBtn.classList.remove("save-mode")
            contentToggleBtn.classList.add("edit-mode")
            isEditingContent = false
        }
    })

    contentCancelBtn.addEventListener("click", () => {
        if (!isEditingContent) {
            return
        }

        contentEditor.classList.add("hidden")
        fileContainer.classList.remove("hidden")
        resetEditorPosition(contentEditor)
        contentToggleBtn.textContent = "Edit"
        contentToggleBtn.classList.remove("save-mode")
        contentToggleBtn.classList.add("edit-mode")
        isEditingContent = false
    })
}


function resetEditorPosition(editor: HTMLTextAreaElement) {
    editor.selectionStart = 0
    editor.selectionEnd = 0
    editor.scrollTop = 0
}

// --- Metadata Editor ---

async function hookMetadataEditorEvents() {
    editBtn.addEventListener("click", async () => {
        if (!isEditingMetadata) {
            enterMetadataEditMode()
        } else {
            await saveMetadataEditMode()
        }
    })
    cancelBtn.addEventListener("click", cancelMetadataEditMode)
}

function enterMetadataEditMode() {
    if (!fileData.metadata) {
        return
    }

    var metadata = fileData.metadata as Record<string, unknown>

    isEditingMetadata = true
    editBtn.textContent = "Save"
    editBtn.classList.remove("edit-mode")
    editBtn.classList.add("save-mode")

    var entries = Object.entries(metadataFieldMap)
    for (var entryIndex = 0; entryIndex < entries.length; entryIndex++) {
        const entry = entries[entryIndex];
        if (entry == null) continue;
        var key = entry[0]
        var valId = entry[1]
        var valueDiv = document.getElementById(valId)
        if (valueDiv == null) {
            continue
        }
        var field = valueDiv.closest(".metadata-field")
        if (field == null) {
            continue
        }
        var fieldDiv = field as HTMLDivElement

        originalVisibility.set(fieldDiv, !fieldDiv.classList.contains("hidden"))
        fieldDiv.classList.remove("hidden")
        valueDiv.style.display = "none"

        var textarea = document.createElement("textarea")
        textarea.className = "metadata-editor"
        textarea.style.width = "100%"
        textarea.style.height = "4em"

        var val = metadata[key]

        if (Array.isArray(val)) {
            textarea.value = val.join("\n")
        } else if (val !== undefined && val !== null) {
            textarea.value = String(val)
        } else {
            textarea.value = ""
        }

        if (valueDiv.parentElement) {
            valueDiv.parentElement.appendChild(textarea)
        }
    }
}

function cancelMetadataEditMode() {
    if (!isEditingMetadata) {
        return
    }

    isEditingMetadata = false
    editBtn.textContent = "Edit"
    editBtn.classList.remove("save-mode")
    editBtn.classList.add("edit-mode")

    var values = Object.values(metadataFieldMap)
    for (var valueIndex = 0; valueIndex < values.length; valueIndex++) {
        const valId = values[valueIndex]
        if (valId == null) continue;
        var valueDiv = document.getElementById(valId)
        if (valueDiv == null) {
            continue
        }
        var field = valueDiv.closest(".metadata-field")
        if (field == null) {
            continue
        }
        var fieldDiv = field as HTMLDivElement
        var textarea = field.querySelector("textarea")

        if (textarea) {
            textarea.remove()
        }
        valueDiv.style.display = ""

        var wasVisible = originalVisibility.get(fieldDiv)
        if (wasVisible) {
            fieldDiv.classList.remove("hidden")
        } else {
            fieldDiv.classList.add("hidden")
        }
    }

    originalVisibility.clear()
}

const arrayFields = [
    "dependencies",
    "preDeployCommands",
    "installCommands",
    "preApplyCommands",
    "reloadCommands",
    "postApplyCommands",
    "postInstallCommands",
];

async function saveMetadataEditMode() {
    if (!fileData.metadata) {
        return
    }

    const metadata = fileData.metadata as Record<string, unknown>

    var entries = Object.entries(metadataFieldMap)
    for (var entryIndex = 0; entryIndex < entries.length; entryIndex++) {
        const entry = entries[entryIndex];
        if (entry == null) continue;
        var key = entry[0]
        var valId = entry[1]
        var valueDiv = document.getElementById(valId)
        if (valueDiv == null) {
            continue
        }
        var field = valueDiv.closest(".metadata-field")
        if (field == null) {
            continue
        }
        var textarea = field.querySelector("textarea")
        if (textarea == null) {
            continue
        }

        var input = textarea.value.trim()

        if (input === "") {
            delete metadata[key]
        } else {
            var currentVal = metadata[key]
            if (Array.isArray(currentVal) || arrayFields.includes(key)) {
                metadata[key] = input.split(/\r?\n/)
            } else {
                metadata[key] = input
            }
        }

        textarea.remove()
        valueDiv.style.display = ""
    }

    // Push updated metadata to server
    const res = await getJSONViaJSON("fs.item.metadata.edit", metadata)

    if (isErr(res)) {
        logError(`saveMetadataEditMode: ${res.error}`, true)
        return
    }

    isEditingMetadata = false
    editBtn.textContent = "Edit"
    editBtn.classList.remove("save-mode")
    editBtn.classList.add("edit-mode")

    populateMetadata(fileData.metadata)
}

// --- INIT ---
async function init() {
    await onDOMReady()

    fileContainer = getElement("file-container")
    filePathSpan = getElement("file-path")
    contentEditor = getElement("file-editor") as HTMLTextAreaElement
    contentToggleBtn = getElement("content-edit-btn")
    contentCancelBtn = getElement("content-cancel-btn")
    editBtn = getElement("metadata-edit-btn")
    cancelBtn = getElement("metadata-cancel-btn")

    initPage()

    var backBtn = document.querySelector<HTMLButtonElement>("#file-header .btn")
    if (backBtn) {
        backBtn.addEventListener("click", (event) => {
            event.preventDefault()
            history.back()
        })
    }

    hookContentEditorEvents()
    hookMetadataEditorEvents()

    const path = getFilePathFromQuery()
    if (!path) {
        logWarning("init: no file path provided in URL")
        fileContainer.textContent = "No file path provided in URL."
        return
    }

    fileData.path = path
    filePathSpan.textContent = path.replace(/^\/+/, "")

    const dataLocation: Result<DownloadLink> = await getJSONViaJSON("fs.item.data.download", { path: path })
    if (isErr(dataLocation)) {
        logError(`init: fetch content link: ${dataLocation.error}`, false)
        return
    }

    const fetchResult = await doFetch(dataLocation.value.downloadLocation, { method: "GET" })
    if (isErr(fetchResult)) {
        logError(`init: fetch content for ${path}: ${fetchResult.error}`, false)
        return
    }
    const textResult = await readText(fetchResult.value)
    if (!isErr(textResult)) {
        fileData.contentLines = textResult.value.split(/\r?\n/)
    }

    const metaResult: Result<FileMetadata> = await getJSONViaJSON("fs.item.metadata.get", { path: path })
    if (isErr(metaResult)) {
        logError(`init: fetch metadata: ${metaResult.error}`, false)
        return
    }
    fileData.metadata = metaResult.value

    populateLines(fileContainer, fileData.contentLines)
    populateMetadata(fileData.metadata)
}

// --- START ---
init()
