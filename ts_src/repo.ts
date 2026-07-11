import { logError, isErr, logWarning, getJSONViaJSON, initRepoDropdown, initVersionInfo } from "./helpers.js";

type WebRepoStatus = {
    staged: WebRepoFileStatus[];
    unstaged: WebRepoFileStatus[];
};

type WebRepoFileStatus = {
    path: string;
    status: string;
};

type WebRepoCommitInfo = {
    shortHash: string;
    fullHash: string;
    date: string;
    authorName: string;
    authorEmail: string;
    numberOfChanges: number;
    filesChanged: WebRepoFileStatus[];
    message: string;
    gpgSignature?: string;
    branches?: string[];
    tags?: string[];
};

let latestStatus: WebRepoStatus | null = null;

interface RepoFileDiffResp {
    files: DiffFile[];
}
interface DiffFile {
    old_path?: string;       // Original path (before rename/move)
    new_path?: string;       // New path (after rename/move)
    change_type: string;     // "added", "modified", "deleted", "renamed", etc.
    is_binary: boolean;      // True if file is binary
    hunks?: DiffHunk[];      // List of changed hunks (if not binary)
}
interface DiffHunk {
    old_start_line: number;  // Start line in the old file
    old_line_count: number;  // Number of lines affected in old file
    new_start_line: number;  // Start line in the new file
    new_line_count: number;  // Number of lines affected in new file
    changes: LineChange[];   // Line-level changes
}
interface LineChange {
    type: "add" | "del" | "context"; // Line change type
    content: string;                 // Line content
}

/* ---------- UI Entry Point ---------- */
let repoUIInitialized = false;

async function initRepoUI() {
    await refreshStatus();
    await refreshHistory();
    hookCommitBox();
}

async function initRepoUIOnce() {
    initVersionInfo();
    initRepoDropdown();

    if (repoUIInitialized) return;
    repoUIInitialized = true;
    try {
        await initRepoUI();
    } catch (err) {
        logError("Failed to initialize repo UI", false);
    }
}

// Run on DOM ready
document.addEventListener("DOMContentLoaded", initRepoUIOnce);

// Run on back/forward navigation if page was persisted (bfcache)
window.addEventListener("pageshow", (event) => {
    if (event.persisted) {
        // re-init only status
        repoUIInitialized = false;
        refreshStatus();
    }
});

/* ---------- Status Panel ---------- */
async function refreshStatus() {
    const result = await getJSONViaJSON<WebRepoStatus>("repo.staging.status");
    if (isErr(result)) {
        logError(`Failed to load repo status: ${result.error}`);
        return;
    }

    latestStatus = result.value;

    renderFileList(
        document.querySelector(".git-panel .git-file-list")!,
        result.value.unstaged,
        "unstaged"
    );
    renderFileList(
        document.querySelectorAll(".git-panel .git-file-list")[1]!,
        result.value.staged,
        "staged"
    );

    hookStageUnstageAll(); // rebind after rendering
}

function renderFileList(
    container: Element,
    files: WebRepoFileStatus[],
    type: "staged" | "unstaged"
) {
    container.innerHTML = "";
    for (const file of files) {
        const li = document.createElement("li");

        const btn = document.createElement("button");
        btn.className =
            "btn small " + (type === "unstaged" ? "btn-stage" : "btn-unstage");
        btn.textContent = type === "unstaged" ? "Stage" : "Unstage";
        btn.onclick = () => toggleStage([file.path], type);

        const link = document.createElement("a");
        link.className = "file-name";
        link.textContent = file.path;
        link.href = `/file.html?path=${encodeURIComponent(file.path)}`;

        const statusSpan = document.createElement("span");
        statusSpan.className = `file-status status-${file.status.toLowerCase()}`;
        statusSpan.textContent = file.status;

        li.appendChild(btn);
        li.appendChild(link);
        li.appendChild(statusSpan);
        container.appendChild(li);
    }
}

async function toggleStage(paths: string[], type: "staged" | "unstaged") {
    const method = type === "unstaged" ? "repo.staging.add" : "repo.staging.remove";
    const payload = { paths };

    const result = await getJSONViaJSON(method, payload);
    if (isErr(result)) {
        logError(`Failed to ${type === "unstaged" ? "stage" : "unstage"}: ${result.error}`);
        return;
    }

    refreshFileList(result.value);
}

function refreshFileList(newStatus: WebRepoStatus) {
    latestStatus = newStatus;
    renderFileList(
        document.querySelector(".git-panel .git-file-list")!,
        newStatus.unstaged,
        "unstaged"
    );
    renderFileList(
        document.querySelectorAll(".git-panel .git-file-list")[1]!,
        newStatus.staged,
        "staged"
    );

    hookStageUnstageAll(); // rebind after rendering
}

/* ---------- Stage/Unstage All ---------- */
function hookStageUnstageAll() {
    const stageAllBtn = document.querySelector<HTMLButtonElement>(
        ".git-panel .btn-stage"
    )!;
    const stageRefreshBtn = document.querySelector<HTMLButtonElement>(
        ".git-panel .btn-refresh"
    )!;
    const unstageAllBtn = document.querySelector<HTMLButtonElement>(
        ".git-panel .btn-unstage"
    )!;

    const updateButtonState = () => {
        if (!latestStatus) return;
        stageAllBtn.disabled = latestStatus.unstaged.length === 0;
        unstageAllBtn.disabled = latestStatus.staged.length === 0;
    };

    stageRefreshBtn.onclick = async () => {
        const result = await getJSONViaJSON("repo.artifacts.refresh");
        if (isErr(result)) {
            logError(`Failed to refresh artifacts: ${result.error}`);
            return;
        }

        latestStatus = result.value;
        // Update UI with the new status
        updateButtonState();
        refreshFileList(result.value);
    };

    stageAllBtn.onclick = async () => {
        if (!latestStatus) return;
        const paths = latestStatus.unstaged.map((f) => f.path);
        if (paths.length === 0) return;

        await toggleStage(paths, "unstaged");
        updateButtonState(); // refresh state after action
    };

    unstageAllBtn.onclick = async () => {
        if (!latestStatus) return;
        const paths = latestStatus.staged.map((f) => f.path);
        if (paths.length === 0) return;

        await toggleStage(paths, "staged");
        updateButtonState(); // refresh state after action
    };

    // initial state
    updateButtonState();
}

/* ---------- Commit Box ---------- */
function hookCommitBox() {
    const textarea = document.getElementById("commit-message") as HTMLTextAreaElement;
    const commitBtn = document.getElementById("commit-button") as HTMLButtonElement;
    const commitDeployBtn = document.getElementById("commit-deploy-button") as HTMLButtonElement;

    const updateButtonState = () => {
        const isEmpty = textarea.value.trim().length === 0;
        commitBtn.disabled = isEmpty;
        commitDeployBtn.disabled = isEmpty;
    };

    textarea.addEventListener("input", updateButtonState);
    updateButtonState(); // set initial state

    const handleCommit = async (redirectToDeploy: boolean) => {
        const message = textarea.value.trim();
        if (!message) return;

        const result = await getJSONViaJSON<any, WebRepoCommitInfo>("repo.commit", { message });
        if (isErr(result)) {
            logError(`Commit failed: ${result.error}`);
            return;
        }

        const commitid = result.value.fullHash;

        if (redirectToDeploy) {
            const params = new URLSearchParams({
                commitid: commitid,
                autorollback: "true"
            });
            const deployLink = "/deployments.html?" + params.toString();
            window.location.href = deployLink;
            return; // skip refreshes
        }

        // Only refresh if we're not redirecting
        textarea.value = "";
        updateButtonState();

        await refreshStatus();
        await refreshHistory();
    };

    commitBtn.addEventListener("click", () => handleCommit(false));
    commitDeployBtn.addEventListener("click", () => handleCommit(true));
}


/* ---------- History Table ---------- */
let currentOffset = 0;
let currentLimit = 10; // default

const rowsSelect = document.getElementById("rows-select") as HTMLSelectElement;
rowsSelect.addEventListener("change", () => {
    currentLimit = parseInt(rowsSelect.value, 10);
    currentOffset = 0; // reset to first page
    refreshHistory(currentOffset, currentLimit);
});

const prevBtn = document.querySelector<HTMLButtonElement>(".pagination button:first-child")!;
const nextBtn = document.querySelector<HTMLButtonElement>(".pagination button:last-child")!;

prevBtn.addEventListener("click", () => {
    currentOffset = Math.max(0, currentOffset - currentLimit);
    refreshHistory(currentOffset, currentLimit);
});

nextBtn.addEventListener("click", () => {
    currentOffset += currentLimit;
    refreshHistory(currentOffset, currentLimit);
});

async function refreshHistory(reqOffset = 0, reqLimit = 10) {
    const result = await getJSONViaJSON<any, WebRepoCommitInfo[]>("repo.commit.history", { limit: reqLimit, offset: reqOffset });
    if (isErr(result)) {
        logError(`Failed to load history: ${result.error}`);
        return;
    }

    const commits = result.value;
    const tbody = document.querySelector<HTMLTableSectionElement>("#commit-history-table tbody");
    if (!tbody) throw new Error("Commit history table body not found");
    tbody.innerHTML = "";

    // Disable/enable buttons based on offset
    prevBtn.disabled = reqOffset === 0;
    nextBtn.disabled = commits.length < reqLimit;

    // Update page info (page number only)
    const pageInfo = document.querySelector<HTMLSpanElement>(".pagination .page-info")!;
    pageInfo.textContent = `Page ${Math.floor(reqOffset / reqLimit) + 1}`;

    // Populate table
    for (let i = 0; i < commits.length; i++) {
        const commit = commits[i];
        const prevCommitHash = i < commits.length - 1 ? commits[i + 1].fullHash : "";

        const tr = document.createElement("tr");
        tr.innerHTML = `
    <td>${commit.shortHash}</td>
    <td>${commit.date}</td>
    <td>${commit.authorName} (${commit.authorEmail})</td>
    <td>${commit.numberOfChanges}</td>
    <td>${commit.message}</td>
    <td>
        <button class="btn btn-show-files">Show</button>
        <button class="btn btn-deploy">Deploy</button>
    </td>
`;
        tbody.appendChild(tr);

        const fileTr = document.createElement("tr");
        fileTr.classList.add("commit-file-row", "hidden");
        const fileTd = document.createElement("td");
        fileTd.setAttribute("colspan", "6");
        const fileListDiv = document.createElement("div");
        fileListDiv.className = "expanded-file-list";

        for (const fileInfo of commit.filesChanged) {
            const fileRow = document.createElement("div");
            fileRow.className = "file-row";

            const pathSpan = document.createElement("span");
            pathSpan.textContent = fileInfo.path;

            const statusContainer = document.createElement("div");
            const statusSpan = document.createElement("span");
            statusSpan.className = `file-status status-${fileInfo.status.toLowerCase()}`;
            statusSpan.textContent = fileInfo.status;
            statusContainer.appendChild(statusSpan);

            const actions = document.createElement("div");
            actions.className = "file-actions";

            const editBtn = document.createElement("button");
            editBtn.textContent = "Edit";
            editBtn.className = "btn";
            editBtn.onclick = () => {
                window.location.href = `/file.html?path=${encodeURIComponent(fileInfo.path)}`;
            };
            if (fileInfo.status === "deleted") {
                editBtn.disabled = true;
                editBtn.classList.add("disabled");
            }

            const diffBtn = document.createElement("button");
            diffBtn.textContent = "Diff";
            diffBtn.className = "btn";
            diffBtn.onclick = async () => {
                let baseHash = prevCommitHash;

                // If base commit is missing, fetch it from API
                if (!baseHash) {
                    try {
                        const qoffset = reqLimit + reqOffset;
                        const res = await getJSONViaJSON<any, WebRepoCommitInfo[]>("repo.commit.history", { limit: 1, offset: qoffset });
                        if (!isErr(res) && res.value.length > 0) {
                            baseHash = res.value[0].fullHash;
                        } else {
                            logWarning("No base commit found; diff will be against empty tree");
                            baseHash = ""; // fallback to empty tree
                        }
                    } catch (err) {
                        logError("Failed to fetch base commit for diff", false);
                        return;
                    }
                }

                const payload = {
                    Path: fileInfo.path,
                    targetCommit: commit.fullHash,
                    baseCommit: baseHash
                };

                const result = await getJSONViaJSON<any, RepoFileDiffResp>("repo.commit.diff", payload);
                if (isErr(result)) {
                    logError(`Failed to fetch diff: ${result.error}`);
                    return;
                }

                showDiffModal(result.value.files);
            };

            actions.appendChild(editBtn);
            actions.appendChild(diffBtn);

            fileRow.appendChild(pathSpan);
            fileRow.appendChild(statusContainer);
            fileRow.appendChild(actions);
            fileListDiv.appendChild(fileRow);
        }

        fileTd.appendChild(fileListDiv);
        fileTr.appendChild(fileTd);
        tbody.appendChild(fileTr);

        const deployBtn = tr.querySelector<HTMLButtonElement>(".btn-deploy")!;
        deployBtn.onclick = () => {
            window.location.href = `/deployments.html?commitid=${encodeURIComponent(commit.fullHash)}`;
        };

        const showBtn = tr.querySelector<HTMLButtonElement>(".btn-show-files")!;
        showBtn.onclick = () => fileTr.classList.toggle("hidden");
    }
}

function showDiffModal(diffInfo: DiffFile[]) {
    // Generate diff text with ±5 lines of context
    let diffText = "";

    for (const file of diffInfo) {
        const fileHeader = `--- ${file.old_path ?? "/dev/null"}\n+++ ${file.new_path ?? "/dev/null"}\n`;
        diffText += fileHeader;

        if (file.is_binary) {
            diffText += `Binary file changed: ${file.new_path ?? file.old_path}\n\n`;
            continue;
        }

        if (!file.hunks) continue;

        for (const hunk of file.hunks) {
            const { changes, old_start_line, new_start_line, old_line_count, new_line_count } = hunk;

            const hunkLines: string[] = [];
            const changeIndexes = changes
                .map((change, idx) => (change.type !== "context" ? idx : null))
                .filter((v): v is number => v !== null);

            const printedIndexes = new Set<number>();

            for (const idx of changeIndexes) {
                const start = Math.max(0, idx - 5);
                const end = Math.min(changes.length, idx + 6);

                // Skip if any index in this range has already been printed
                let alreadyPrinted = false;
                for (let i = start; i < end; i++) {
                    if (printedIndexes.has(i)) {
                        alreadyPrinted = true;
                        break;
                    }
                }
                if (alreadyPrinted) continue;

                // Mark these indexes as printed
                for (let i = start; i < end; i++) {
                    printedIndexes.add(i);
                }

                // Add hunk header
                hunkLines.push(`@@ -${old_start_line},${old_line_count} +${new_start_line},${new_line_count} @@`);

                for (let i = start; i < end; i++) {
                    const change = changes[i];
                    const prefix = change.type === "add" ? "+" : change.type === "del" ? "-" : " ";
                    hunkLines.push(`${prefix}${change.content}`);
                }

                hunkLines.push(""); // blank line between chunks
            }

            diffText += hunkLines.join("\n") + "\n";
        }
    }

    // --- Modal UI rendering (same as before) ---
    const overlay = document.createElement("div");
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
    });

    const modal = document.createElement("div");
    Object.assign(modal.style, {
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
    });

    const textContainer = document.createElement("div");
    textContainer.textContent = diffText;

    const buttons = document.createElement("div");
    Object.assign(buttons.style, {
        display: "flex",
        justifyContent: "center",
        marginTop: "1rem",
    });

    const backBtn = document.createElement("button");
    backBtn.textContent = "Back";
    Object.assign(backBtn.style, {
        padding: "0.3rem 1.2rem",
        fontSize: "1rem",
        cursor: "pointer",
        backgroundColor: "#333",
        color: "#ddd",
        border: "none",
        borderRadius: "0.25rem",
        transition: "background 0.3s",
    });

    backBtn.onmouseenter = () => (backBtn.style.backgroundColor = "#444");
    backBtn.onmouseleave = () => (backBtn.style.backgroundColor = "#333");
    backBtn.onclick = () => document.body.removeChild(overlay);

    buttons.appendChild(backBtn);
    modal.appendChild(textContainer);
    modal.appendChild(buttons);
    overlay.appendChild(modal);
    document.body.appendChild(overlay);
}
