import { logError, isErr, Result, logWarning, getJSONViaJSON, initRepoDropdown, initVersionInfo } from "./helpers.js";
import { showModal } from "./modal.js";

interface WebPathReq {
    path: string;
}

interface WebFileMetadata {
    path: string;
    type: "file" | "directory";
    size?: number;
    ownerName: string;
    groupName: string;
    permissions: string;
    lastModified?: string;
}

interface WebFilesList {
    directory: string;
    entries: WebFileMetadata[];
}

interface WebFileOp {
    path: string;
    type: "file" | "directory";
    recursive?: boolean; // for directories
}

interface WebFileMove {
    sourcePath: string;
    destinationPath: string;
    deleteSource: boolean;
    overwriteDestination: boolean;
}

interface SearchReq {
    path: string;
    query: string;
    querytype?: "exact" | "contains" | "prefix" | "suffix";
    filetype?: "all" | "file" | "directory";
    depth: number;
}

interface SearchResults {
    path: string;
    query: string;
    queryType: string;
    fileType: string;
    depth: number;
    matchCount: number;
    matches: WebFileMetadata[];
}

// State
let currentPath = "/";
let allFiles: WebFileMetadata[] = [];
let currentPage = 1;

// DOM references
const tableBody = document.querySelector<HTMLTableSectionElement>("tbody")!;
const paginationInfo = document.querySelector<HTMLSpanElement>(".page-info")!;
const paginationDiv = document.querySelector<HTMLDivElement>(".pagination")!;
const buttons = Array.from(paginationDiv.querySelectorAll<HTMLButtonElement>(".btn"));
const [prevButton, nextButton] = buttons;
const pathHeader = document.getElementById("current-path")!;

// Helpers
function normalizePath(path: string): string {
    if (!path.startsWith("/")) path = "/" + path;
    return path.replace(/\/+/g, "/");
}

// Delete a file or directory
async function deleteEntry(entry: WebFileOp): Promise<Result<void>> {
    const op: WebFileOp = {
        path: entry.path,
        type: entry.type,
        recursive: entry.type === "directory", // only recursive for directories
    };
    const res = await getJSONViaJSON("fs.item.delete", op);
    if (isErr(res)) {
        logError(`Failed to delete ${entry.path}: ${res.error}`, false);
    }
    return res;
}

// Fetch directory listing using helper
async function fetchFiles(req: WebPathReq): Promise<WebFileMetadata[]> {
    const res: Result<WebFilesList> = await getJSONViaJSON("fs.directory.list", { path: req.path });
    if (isErr(res)) {
        logError(`Failed to fetch directory '${req.path}': ${res.error}`, false);
        return [];
    }

    const entries = Array.isArray(res.value) ? res.value : [];

    // Notify of nothing received, probably error
    if (entries.length === 0) {
        logError(`No files received for directory '${req.path}'`, false);
    }

    return entries;
}

async function searchFiles(req: SearchReq): Promise<SearchResults> {
    const params = new URLSearchParams();

    const res: Result<SearchResults> = await getJSONViaJSON("fs.item.search", {
        path: req.path,
        query: req.query,
        searchType: req.querytype,
        fileType: req.filetype,
        depth: req.depth
    });
    if (isErr(res)) {
        logError(`Failed search`, false);
        return {
            path: req.path,
            query: req.query,
            queryType: req.querytype ?? "contains",
            fileType: req.filetype ?? "all",
            depth: req.depth,
            matchCount: 0,
            matches: [],
        };
    }

    return res.value
}

// Move a file or directory
export async function moveEntry(move: WebFileMove): Promise<Result<void>> {
    const res = await getJSONViaJSON("fs.item.move", move);
    if (isErr(res)) {
        logError(`Failed to move '${move.sourcePath}' to '${move.destinationPath}': ${res.error}`, false);
    }
    return res;
}

const createBtn = document.getElementById("create-btn")!;

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
            if (!inputValue) return;

            const type = (selectValues?.["create-type-select"] ?? "file") as "file" | "directory";

            const nPath = normalizePath(`${currentPath}/${inputValue}`)
            const res = await getJSONViaJSON("fs.item.new", {
                path: normalizePath(`${currentPath}/${inputValue}`),
                type: type,
                recursive: false,
            });
            if (isErr(res)) {
                logError(`Failed to create ${type} '${nPath}': ${res.error}`, false);
            } else {
                allFiles = await fetchFiles({ path: currentPath });
                renderPage();
            }
        }
    });
});

// Load directory
async function loadDirectory(path: string): Promise<void> {
    currentPage = 1;
    currentPath = normalizePath(path);
    allFiles = await fetchFiles({ path: currentPath });
    isSearchResults = false;
    renderPage();
}

let searchOptions = {
    matchType: "contains", // contains|exact|prefix|suffix
    fileType: "all",       // all|file|directory
    depth: 10              // number >= 0
};
let isSearchResults = false;
const searchInput = document.getElementById("search-input") as HTMLInputElement;
const searchBtn = document.getElementById("search-btn") as HTMLButtonElement;
const clearSearchBtn = document.getElementById("clear-search-btn") as HTMLButtonElement;
const searchCog = document.getElementById("search-cog") as HTMLElement;

async function performSearch(query: string) {
    if (!query.trim()) return;

    const results = await searchFiles({
        path: currentPath,
        query,
        querytype: searchOptions.matchType as any,
        filetype: searchOptions.fileType as any,
        depth: searchOptions.depth,
    });

    allFiles = Array.isArray(results.matches) ? results.matches : [];
    if (allFiles.length === 0) {
        logWarning(`Search did not return any results`);
    }
    isSearchResults = true;
    currentPage = 1;
    renderPage();
}

// Enter key triggers search
searchInput.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
        e.preventDefault();
        performSearch(searchInput.value);
    }
});

// Search button click triggers search
searchBtn.addEventListener("click", () => {
    performSearch(searchInput.value);
});

// Clear search resets listing
clearSearchBtn.addEventListener("click", async () => {
    allFiles = await fetchFiles({ path: currentPath });
    isSearchResults = false;
    currentPage = 1;
    renderPage();
    searchInput.value = "";
});

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
            // Save selected values globally
            searchOptions.matchType = (selectValues?.["select-query-type"] ?? "contains") as typeof searchOptions.matchType;
            searchOptions.fileType = (selectValues?.["select-file-type"] ?? "all") as typeof searchOptions.fileType;

            let parsedDepth = parseInt(inputValues?.["input-depth"] ?? "10", 10);
            if (isNaN(parsedDepth) || parsedDepth < 0) {
                alert("Depth must be a non-negative number.");
                return false; // prevent modal from closing
            }
            searchOptions.depth = parsedDepth;
            return true; // close modal
        }
    });
});

// Add row count dropdown
const rowSelect = document.getElementById("rows-select") as HTMLSelectElement;
let pageSize = parseInt(rowSelect.value, 10); // initialize from selection

// Update pageSize when user changes selection
rowSelect.addEventListener("change", () => {
    pageSize = parseInt(rowSelect.value, 10);
    currentPage = 1; // reset to first page
    renderPage();
});

// Update your existing pagination prev/next buttons
prevButton.addEventListener("click", () => {
    if (currentPage > 1) {
        currentPage--;
        renderPage();
    }
});
nextButton.addEventListener("click", () => {
    const totalPages = Math.ceil(allFiles.length / pageSize);
    if (currentPage < totalPages) {
        currentPage++;
        renderPage();
    }
});

const refreshBtn = document.getElementById("refresh-btn") as HTMLButtonElement;
refreshBtn.addEventListener("click", async () => {
    // Reload the directory list
    allFiles = await fetchFiles({ path: currentPath });
    isSearchResults = false;
    renderPage();
});

// Render table page
function renderPage(): void {
    tableBody.innerHTML = "";
    const totalPages = Math.max(1, Math.ceil(allFiles.length / pageSize));
    if (currentPage > totalPages) currentPage = totalPages;

    const pageFiles = allFiles.slice((currentPage - 1) * pageSize, currentPage * pageSize);

    pageFiles.forEach(file => {
        const row = document.createElement("tr");

        // Name cell
        const nameCell = document.createElement("td");
        const isSearchResult = !file.path.startsWith(normalizePath(currentPath + "/"));
        const displayName = isSearchResults
            ? file.path            // full path for search results
            : file.path.split("/").filter(Boolean).pop() || file.path;  // just name for normal listing

        const link = document.createElement("span");
        link.textContent = displayName;
        link.style.cursor = "pointer";
        link.classList.add("breadcrumb-segment");
        if (file.type === "directory") link.classList.add("directory");

        link.addEventListener("click", async () => {
            if (file.type === "directory") {
                await loadDirectory(normalizePath(file.path));
            } else {
                window.location.href = `/file.html?path=${encodeURIComponent(normalizePath(file.path))}`;
            }
        });

        nameCell.appendChild(link);
        row.appendChild(nameCell);

        // Other cells
        row.appendChild(createCell(file.permissions));
        row.appendChild(createCell(file.ownerName));
        row.appendChild(createCell(file.groupName));
        row.appendChild(
            createCell(file.size === 0 || file.size == null ? "-" : file.size.toString())
        );
        row.appendChild(createCell(file.lastModified == null ? "-" : file.lastModified))

        // Actions cell
        const actionsCell = document.createElement("td");
        actionsCell.classList.add("actions-cell");

        const moveBtn = document.createElement("button");
        moveBtn.textContent = "Move";
        moveBtn.classList.add("btn", "btn-move");
        moveBtn.onclick = () => {
            showModal({
                message: `Move/Copy '${file.path}' to:`,
                inputPlaceholder: "Destination path",
                confirmText: "Submit",
                cancelText: "Cancel",
                checkboxes: [
                    { id: "deleteSource", label: "Delete source", default: false },
                    { id: "overwriteDestination", label: "Overwrite destination", default: false }
                ],
                onConfirm: async (inputValue, checkboxValues) => {
                    if (!inputValue) return;

                    await moveEntry({
                        sourcePath: file.path,
                        destinationPath: inputValue,
                        deleteSource: checkboxValues?.deleteSource ?? false,
                        overwriteDestination: checkboxValues?.overwriteDestination ?? false
                    });

                    // Refresh directory listing
                    allFiles = await fetchFiles({ path: currentPath });
                    renderPage();
                }
            });
        };

        const delBtn = document.createElement("button");
        delBtn.textContent = "Delete";
        delBtn.className = "btn";
        delBtn.onclick = () => {
            const fileName = file.path.split("/").pop()!;

            showModal({
                message: `Confirm deletion of '${file.path}':`,
                inputPlaceholder: `Type '${fileName}' to confirm`, // always require name input
                checkboxes: file.type === "directory"
                    ? [{ id: "recursive", label: "Recursive", default: false }]
                    : [],
                confirmText: "Delete",
                cancelText: "Cancel",
                onConfirm: async (inputValue, checkboxValues) => {
                    if (inputValue !== fileName) {
                        alert("Name does not match. Deletion cancelled.");
                        return;
                    }

                    // For this modal, only one checkbox exists (if directory)
                    const recursive = checkboxValues?.[0] ?? false;

                    const res = await deleteEntry({
                        path: file.path,
                        type: file.type,
                        recursive,
                    });

                    if (isErr(res)) {
                        logError(`Failed to delete '${file.path}': ${res.error}`, false);
                    } else {
                        allFiles = allFiles.filter(f => f.path !== file.path);
                        renderPage();
                    }
                },
            });
        };

        actionsCell.appendChild(moveBtn);
        actionsCell.appendChild(delBtn);
        row.appendChild(actionsCell);

        tableBody.appendChild(row);
    });

    updatePathHeader();

    prevButton.disabled = currentPage === 1;
    nextButton.disabled = currentPage === totalPages;
    paginationInfo.textContent = `Page ${currentPage} of ${totalPages}`;
}
function createCell(content: string): HTMLTableCellElement {
    const td = document.createElement("td");
    td.textContent = content;
    return td;
}

// Breadcrumb
function updatePathHeader() {
    pathHeader.innerHTML = "";
    const segments = currentPath.split("/").filter(Boolean);

    const rootLink = document.createElement("span");
    rootLink.textContent = "/";
    rootLink.classList.add("breadcrumb-segment");
    rootLink.addEventListener("click", async () => await loadDirectory("/"));
    pathHeader.appendChild(rootLink);

    let builtPath = "";
    segments.forEach((seg, idx) => {
        const sep = document.createElement("span");
        sep.textContent = idx > 0 ? " / " : " ";
        sep.classList.add("breadcrumb-separator");
        pathHeader.appendChild(sep);

        const segLink = document.createElement("span");
        segLink.textContent = seg;
        segLink.classList.add("breadcrumb-segment");

        const segPath = normalizePath(builtPath + "/" + seg);
        segLink.addEventListener("click", async () => await loadDirectory(segPath));

        pathHeader.appendChild(segLink);

        builtPath += "/" + seg;
    });

    const copyBtn = document.createElement("button");
    copyBtn.textContent = "📋";
    copyBtn.classList.add("btn");
    copyBtn.title = "Copy full path";

    copyBtn.addEventListener("click", async (e) => {
        try {
            await navigator.clipboard.writeText(currentPath);

            // Copy success feedback
            const check = document.createElement("span");
            check.textContent = "✔";
            check.style.position = "fixed";
            check.style.left = `${e.clientX}px`;
            check.style.top = `${e.clientY}px`;
            check.style.transform = "translate(-50%, -50%)";
            check.style.color = "#4caf50";
            check.style.fontSize = "1.6rem";
            check.style.pointerEvents = "none";
            check.style.opacity = "1";
            check.style.transition = "opacity 0.5s ease, transform 0.5s ease";
            document.body.appendChild(check);

            // Animate: float up and fade out
            requestAnimationFrame(() => {
                check.style.transform = "translate(-50%, -80%)";
                check.style.opacity = "0";
            });

            // Remove after animation
            setTimeout(() => check.remove(), 1000);
        } catch (err) {
            logError("Failed to copy path to clipboard: " + err, false)
        }
    });
    pathHeader.appendChild(copyBtn);
}

// Init
initVersionInfo();
initRepoDropdown();
loadDirectory(currentPath);
