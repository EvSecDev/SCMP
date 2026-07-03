import { sendData, logError, isErr, Result, getJSONViaJSON, initRepoDropdown } from "./helpers.js";

// --- DOM ELEMENTS ---
let fileContainer: HTMLElement;
let fileMetadataDiv: HTMLElement;
let filePathSpan: HTMLElement;
let contentEditor: HTMLTextAreaElement;
let contentToggleBtn: HTMLElement;
let contentCancelBtn: HTMLElement;
let editBtn: HTMLElement;
let cancelBtn: HTMLElement;
let metadataFields: HTMLDivElement[];

let isEditingContent = false;
let isEditingMetadata = false;
const originalVisibility = new Map<HTMLDivElement, boolean>();

// --- Utility: Wait for DOM ---
function onDOMReady(): Promise<void> {
    return new Promise(resolve => {
        if (document.readyState === "loading") {
            document.addEventListener("DOMContentLoaded", () => resolve());
        } else {
            resolve();
        }
    });
}

function getFilePathFromQuery(): string | null {
    return new URLSearchParams(window.location.search).get("path");
}

/** Type for metadata returned by the API */
interface WebFileMetadata {
    path: string;
    type?: string;
    size?: number;
    ownerName?: string;
    groupName?: string;
    permissions?: string;
    lastModified?: string;
    externalContentLocation?: string;
    symbolicLinkTarget?: string;
    dependencies?: string[];
    preDeployCommands?: string[];
    installCommands?: string[];
    preApplyCommands?: string[];
    postApplyCommands?: string[];
    postInstallCommands?: string[];
    reloadCommands?: string[];
    reloadGroup?: string;
}

const metadataFieldMap: { [key: string]: string } = {
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
};

interface FileDataModel {
    path: string;
    contentLines: string[];
    metadata: WebFileMetadata | null;
}

// Hold this files information here for source of truth
const fileData: FileDataModel = {
    path: "",
    contentLines: [],
    metadata: null
};

/** Populate the file lines in the container */
function populateLines(container: HTMLElement, lines: string[]) {
    container.innerHTML = "";
    lines.forEach(lineText => {
        const lineDiv = document.createElement("div");
        lineDiv.textContent = lineText === "" ? "\u00a0" : lineText; // preserve empty lines
        container.appendChild(lineDiv);
    });
}

/** Populate metadata fields */
function populateMetadata(metadata: WebFileMetadata | null) {
    Object.entries(metadataFieldMap).forEach(([key, valId]) => {
        const valueDiv = document.getElementById(valId);
        const container = valueDiv?.parentElement;
        if (!valueDiv || !container) return;

        const value = (metadata as any)?.[key];

        if (value === undefined || value === null || (Array.isArray(value) && value.length === 0)) {
            container.classList.add("hidden");
            return;
        }

        container.classList.remove("hidden");

        if (Array.isArray(value)) {
            valueDiv.innerHTML = "";
            value.forEach(item => {
                const itemDiv = document.createElement("div");
                itemDiv.textContent = String(item);
                valueDiv.appendChild(itemDiv);
            });
        } else {
            valueDiv.textContent = String(value);
        }
    });
}

// --- Content Editor ---

function hookContentEditorEvents() {
    let isSaving = false;  // flag to prevent multiple saves

    contentToggleBtn.addEventListener("click", async () => {
        if (isSaving) return; // prevent if already saving

        if (!isEditingContent) {
            // Enter edit mode
            contentEditor.value = fileData.contentLines.join("\n");

            fileContainer.classList.add("hidden");
            contentEditor.classList.remove("hidden");
            contentEditor.focus();

            resetEditorPosition(contentEditor);

            contentToggleBtn.textContent = "Save";
            contentToggleBtn.classList.remove("edit-mode");
            contentToggleBtn.classList.add("save-mode");
            isEditingContent = true;
        } else {
            isSaving = true; // block concurrent saves

            fileData.contentLines = contentEditor.value.split(/\r?\n/);

            const contentPayload = fileData.contentLines.join("\n");

            const path = filePathSpan.textContent || "";

            const res = await sendData(`/data-store/upload`, "POST", contentPayload, true);
            if (isErr(res)) {
                isSaving = false;
                logError(`Failed to upload file content: ${res.error}`, true);
                return;
            }

            if (res.value === "" || res.value === undefined || res.value === null) {
                isSaving = false;
                logError(`Server did not respond with data ID for upload`, false);
                return;
            }

            const saveRes = await getJSONViaJSON("fs.item.data.save", { path: path, dataID: res.value });

            isSaving = false;

            if (isErr(saveRes)) {
                isSaving = false;
                logError(`Failed to save file content`, true);
                return;
            }

            populateLines(fileContainer, fileData.contentLines);

            contentEditor.classList.add("hidden");
            fileContainer.classList.remove("hidden");

            resetEditorPosition(contentEditor);

            contentToggleBtn.textContent = "Edit";
            contentToggleBtn.classList.remove("save-mode");
            contentToggleBtn.classList.add("edit-mode");
            isEditingContent = false;
        }
    });

    contentCancelBtn.addEventListener("click", () => {
        if (!isEditingContent) return;

        contentEditor.classList.add("hidden");
        fileContainer.classList.remove("hidden");

        resetEditorPosition(contentEditor);

        contentToggleBtn.textContent = "Edit";
        contentToggleBtn.classList.remove("save-mode");
        contentToggleBtn.classList.add("edit-mode");
        isEditingContent = false;
    });
}


function resetEditorPosition(editor: HTMLTextAreaElement) {
    editor.selectionStart = 0;
    editor.selectionEnd = 0;
    editor.scrollTop = 0;
}

// --- Metadata Editor ---

async function hookMetadataEditorEvents() {
    editBtn.addEventListener("click", async () => {
        if (!isEditingMetadata) {
            enterMetadataEditMode();
        } else {
            await saveMetadataEditMode();
        }
    });
    cancelBtn.addEventListener("click", cancelMetadataEditMode);
}

function enterMetadataEditMode() {
    if (!fileData.metadata) return;

    const metadata = fileData.metadata; // <-- now it's non-null

    isEditingMetadata = true;
    editBtn.textContent = "Save";
    editBtn.classList.remove("edit-mode");
    editBtn.classList.add("save-mode");

    Object.entries(metadataFieldMap).forEach(([key, valId]) => {
        const field = document.getElementById(valId)?.closest(".metadata-field") as HTMLDivElement;
        const valueDiv = document.getElementById(valId)!;
        if (!field || !valueDiv) return;

        originalVisibility.set(field, !field.classList.contains("hidden"));
        field.classList.remove("hidden");

        valueDiv.style.display = "none";

        const textarea = document.createElement("textarea");
        textarea.className = "metadata-editor";
        textarea.style.width = "100%";
        textarea.style.height = "4em";

        const val = metadata[key as keyof WebFileMetadata]; // now safe

        textarea.value = Array.isArray(val)
            ? val.join("\n")
            : val !== undefined && val !== null
                ? String(val)
                : "";

        valueDiv.parentElement!.appendChild(textarea);
    });
}

function cancelMetadataEditMode() {
    if (!isEditingMetadata) return;

    isEditingMetadata = false;
    editBtn.textContent = "Edit";
    editBtn.classList.remove("save-mode");
    editBtn.classList.add("edit-mode");

    Object.entries(metadataFieldMap).forEach(([_, valId]) => {
        const field = document.getElementById(valId)?.closest(".metadata-field") as HTMLDivElement;
        const valueDiv = document.getElementById(valId)!;
        const textarea = field.querySelector("textarea");

        if (textarea) textarea.remove();
        valueDiv.style.display = "";

        if (originalVisibility.get(field)) {
            field.classList.remove("hidden");
        } else {
            field.classList.add("hidden");
        }
    });

    originalVisibility.clear();
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
    if (!fileData.metadata) return;

    const metadata = fileData.metadata;

    isEditingMetadata = false;
    editBtn.textContent = "Edit";
    editBtn.classList.remove("save-mode");
    editBtn.classList.add("edit-mode");

    Object.entries(metadataFieldMap).forEach(([key, valId]) => {
        const field = document.getElementById(valId)?.closest(".metadata-field") as HTMLDivElement;
        const valueDiv = document.getElementById(valId)!;
        const textarea = field.querySelector("textarea");
        if (!textarea) return;

        const input = textarea.value.trim();

        if (input === "") {
            delete (fileData.metadata as any)[key];
        } else {
            // For known array fields, always send array, even if one item
            if (Array.isArray((fileData.metadata as any)[key]) || arrayFields.includes(key)) {
                (fileData.metadata as any)[key] = input.split(/\r?\n/);
            } else {
                (fileData.metadata as any)[key] = input;
            }
        }

        textarea.remove();
        valueDiv.style.display = "";
    });

    // Push updated metadata to server
    const res = await getJSONViaJSON("fs.item.metadata.edit", metadata);

    if (isErr(res)) {
        logError(`Failed to update metadata for '${metadata.path}': ${res.error}`, true);
    }

    populateMetadata(fileData.metadata);
}

// --- INIT ---
async function init() {
    await onDOMReady();

    initRepoDropdown();

    const backBtn = document.querySelector<HTMLAnchorElement>('#file-header .btn');
    if (backBtn) {
        backBtn.addEventListener('click', (event) => {
            event.preventDefault(); // prevent the default link behavior
            history.back();
        });
    }

    // DOM references
    fileContainer = document.getElementById("file-container")!;
    fileMetadataDiv = document.getElementById("file-metadata")!;
    filePathSpan = document.getElementById("file-path")!;
    contentEditor = document.getElementById("file-editor") as HTMLTextAreaElement;
    contentToggleBtn = document.getElementById("content-edit-btn")!;
    contentCancelBtn = document.getElementById("content-cancel-btn")!;
    editBtn = document.getElementById("metadata-edit-btn")!;
    cancelBtn = document.getElementById("metadata-cancel-btn")!;
    metadataFields = Array.from(document.querySelectorAll<HTMLDivElement>("#file-metadata .metadata-field"));

    hookContentEditorEvents();
    await hookMetadataEditorEvents();

    const path = getFilePathFromQuery();
    if (!path) {
        fileContainer.textContent = "No file path provided in URL.";
        return;
    }

    fileData.path = path;
    filePathSpan.textContent = path.replace(/^\/+/, "");

    interface DownloadResponse {
        downloadLocation: string;
    }

    const dataLocation: Result<DownloadResponse> = await getJSONViaJSON("fs.item.data.download", { path: path });
    if (isErr(dataLocation)) {
        logError(`Failed to fetch content link for '${path}': ${dataLocation.error}`, false);
        return "";
    }

    const dataResult = await fetch(dataLocation.value.downloadLocation);
    if (!dataResult.ok) {
        logError(`Failed to fetch content for '${path}'`, false);
        return "";
    }
    const text = await dataResult.text();
    fileData.contentLines = text.split(/\r?\n/);

    const metaResult: Result<WebFileMetadata> = await getJSONViaJSON("fs.item.metadata.get", { path: path });
    if (isErr(metaResult)) {
        logError(`Failed to fetch metadata for '${path}': ${metaResult.error}`, false);
        return null;
    }
    fileData.metadata = metaResult.value;

    populateLines(fileContainer, fileData.contentLines);
    populateMetadata(fileData.metadata);
}

// --- START ---
init(); // Safe to call immediately — will wait for DOM
