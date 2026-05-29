import { getJSONViaJSON, logError, isErr, initRepoDropdown, Result, logWarning } from "./helpers.js";
import { setupOverrideHostsDropdown, selectedHosts } from "./dropdown.js";
import { showModal } from "./modal.js";

type WebDeployStart = {
    mode: string;
    type: string;
    options: {
        allowDeletions: boolean;
        runInstall: boolean;
        disableReloads: boolean;
        disableSudo: boolean;
        ignoreHostState: boolean;
        force: boolean;
        autoCommitRollbackEnabled: boolean;
        commitID: string;
        fileOverride: string;
        hostOverride: string;
        runAsUser: string;
        maxSSHConnections: number;
        maxSSHChannels: number;
        maxCommandRuntime: number;
        verbosity: number;
    };
};

type DeployStatus = {
    deploymentID: string;
    status: "started" | "running" | "parsing output" | "finished";
    pending: boolean;
    pendingAction: PromptReq[];
}

type PromptReq = {
    associatedDataID: string;
    promptID: string;
    title: string;
    details: string;
    type: string;
}

type PromptAnswer = {
    associatedDataID: string;
    promptID: string;
    encodedData: string;
}

type DeploymentResult = {
    deploymentID: string;
    status: "started" | "running" | "parsing output" | "finished";
    rawOutput: string;
    summary: DeploymentSummary;
};

type DeploymentSummary = {
    Status: string;
    "Start-Time": string;
    "End-Time": string;
    "Elapsed-Time": string;
    "Transferred-Size": string;
    Counters: {
        Hosts: number;
        Items: number;
        "Hosts-Completed": number;
        "Items-Completed": number;
        "Hosts-Failed": number;
        "Items-Failed": number;
    };
    "Deployment-Commit-Hash": string;
    Hosts?: HostSummary[];
};

type HostSummary = {
    Name: string;
    Status?: string;
    "Error-Message"?: string;
    "Total-Items"?: number;
    "Transferred-Size"?: string;
    Items?: ItemSummary[];
};

type ItemSummary = {
    Name: string;
    "Deployment-Action": string;
    Status?: string;
    "Error-Message"?: string;
};

async function handleDeployClick() {
    const deployBtn = document.querySelector(".deploy-final-btn") as HTMLButtonElement;
    const spinner = document.querySelector(".deploy-btn-wrapper .spinner") as HTMLElement;

    try {
        // Fade + disable button and show spinner
        deployBtn.disabled = true;
        spinner.classList.remove("hidden");

        // Step 1: Read inputs
        const mode =
            document.querySelector<HTMLInputElement>("input[name='deploy-mode']:checked")?.id.replace("mode-", "") || "diff";
        const type =
            document.querySelector<HTMLInputElement>("input[name='opt-type']:checked")?.id.replace("type-", "") || "preview";

        const getCheckbox = (id: string): boolean => {
            const el = document.getElementById(id);
            return el instanceof HTMLInputElement ? el.checked : false;
        };

        const getSelectedHostsCSV = (): string => Array.from(selectedHosts).join(",");

        const getInputValueById = (id: string): string =>
            (document.getElementById(id) as HTMLInputElement)?.value || "";

        // Step 2: Construct deployment options
        const opts: WebDeployStart["options"] = {
            allowDeletions: getCheckbox("opt-allow-deletions"),
            runInstall: getCheckbox("opt-run-install"),
            disableReloads: getCheckbox("opt-disable-reloads"),
            disableSudo: getCheckbox("opt-disable-sudo"),
            ignoreHostState: getCheckbox("opt-ignore-state"),
            force: getCheckbox("opt-force"),
            autoCommitRollbackEnabled: getCheckbox("opt-autorollback"),
            commitID: getInputValueById("commitid"),
            fileOverride: getInputValueById("overridefiles"),
            hostOverride: getSelectedHostsCSV(),
            runAsUser: getInputValueById("runasuser"),
            maxSSHConnections: parseInt(getInputValueById("maxsshconns")) || 10,
            maxSSHChannels: parseInt(getInputValueById("maxsshchan")) || 5,
            maxCommandRuntime: parseInt(getInputValueById("cmdtimeout")) || 180,
            verbosity: parseInt(getInputValueById("verbosity")) || 1
        };

        // Step 3: Construct payload
        const payload: WebDeployStart = {
            mode,
            type,
            options: opts
        };

        // Step 4: Send POST request
        const deployRes = await getJSONViaJSON<WebDeployStart, DeployStatus>("deployment.start", payload);

        if (isErr(deployRes)) {
            return logError("Failed to start deployment");
        }

        const deploymentID = deployRes.value.deploymentID;

        // Step 5: Poll for status
        await pollDeploymentStatus(deploymentID);

    } catch (err) {
        logError(`Unexpected error during deployment: ${err}`);
    } finally {
        // Re-enable and un-fade button, hide spinner
        deployBtn.disabled = false;
        spinner.classList.add("hidden");
    }
}

async function pollDeploymentStatus(reqID: string): Promise<void> {
    const abortBtn = document.querySelector(".deploy-abort-btn") as HTMLButtonElement;
    let aborted: boolean = false;
    const abortHandler = async () => {
        const res = await getJSONViaJSON<
            { deploymentID: string; stopRequested: boolean },
            { status: string }
        >("deployment.abort", { deploymentID: reqID, stopRequested: true });

        if (isErr(res)) {
            logError("Failed to abort deployment: " + res.error, false);
            return;
        }
        aborted = true;
    };

    // Add the listener when polling starts
    abortBtn.addEventListener("click", abortHandler); abortBtn?.removeAttribute("disabled");
    abortBtn.removeAttribute("disabled");

    return new Promise<void>((resolve) => {
        const poll = async () => {
            if (aborted) {
                abortBtn.setAttribute("disabled", "");
                abortBtn.removeEventListener("click", abortHandler);
                logWarning("Aborted deployment " + reqID);
                resolve();
                return;
            }

            const statusRes = await getJSONViaJSON<{ deploymentID: string }, DeployStatus>(
                "deployment.status",
                { deploymentID: reqID }
            );

            if (isErr(statusRes)) {
                logError("Failed to fetch deployment status");
                abortBtn.setAttribute("disabled", "");
                abortBtn.removeEventListener("click", abortHandler);
                resolve();
                return;
            }

            const status = statusRes.value.status;

            if (status === "finished") {
                await fetchDeploymentOutput(reqID);  // ensure output is fetched before finishing
                abortBtn.setAttribute("disabled", "");
                abortBtn.removeEventListener("click", abortHandler);
                resolve();
            } else if (statusRes.value.pending) {
                const err = await askUser(statusRes.value.pendingAction)
                if (err.trim() != "") {
                    logError("Failed prompt answer: " + err)
                    abortBtn.setAttribute("disabled", "");
                    abortBtn.removeEventListener("click", abortHandler);
                    resolve();
                }
                setTimeout(poll, 2000);
            } else {
                setTimeout(poll, 2000); // poll again in 2 seconds
            }
        };

        poll(); // kick off the polling
    });
}

async function askUser(prompts: PromptReq[]): Promise<string> {
    let answers: PromptAnswer[] = [];
    let err = "";

    for (const prompt of prompts) {
        let hideInputText: boolean = false;
        if (prompt.type === "secret") {
            hideInputText = true;
        }

        let userInput: string | undefined;
        await new Promise<void>((resolve) => {
            showModal({
                message: prompt.title,
                inputPlaceholder: "Input",
                confirmText: "Enter",
                cancelText: "Cancel",
                hideInput: hideInputText,
                onConfirm: (inputValue, checkboxes, selects, inputs) => {
                    userInput = inputValue;
                    resolve();
                },
            });
        });

        if (userInput == undefined) {
            err = "Invalid input for prompt: " + prompt.title;
            return err;
        }

        console.log(btoa(userInput))

        const newAnswer: PromptAnswer = {
            associatedDataID: prompt.associatedDataID,
            promptID: prompt.promptID,
            encodedData: btoa(userInput),
        }
        answers.push(newAnswer);
    }

    const ansResp = await getJSONViaJSON<PromptAnswer[], { status: string }>("user.pending.prompt.answer", answers);
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

async function fetchDeploymentOutput(reqID: string) {
    const outputEl = document.querySelector(".deploy-output") as HTMLElement;
    const summaryEl = document.getElementById("deploy-summary")!;
    const outputRes = await getJSONViaJSON<object, DeploymentResult>("deployment.result", { deploymentID: reqID });

    if (!outputRes.ok) {
        outputEl.textContent = "(No output)";
        return;
    }

    const result = outputRes.value;
    outputEl.textContent = result.rawOutput;

    // --- Populate summary UI ---
    const summary = result.summary;

    // Fill in top status block
    document.querySelector('[data-field="status-text"]')!.textContent = summary.Status;
    document.querySelector('[data-field="start-time"]')!.textContent = summary["Start-Time"];
    document.querySelector('[data-field="end-time"]')!.textContent = summary["End-Time"];
    document.querySelector('[data-field="elapsed-time"]')!.textContent = summary["Elapsed-Time"];
    document.querySelector('[data-field="transferred-size"]')!.textContent = summary["Transferred-Size"];
    document.querySelector('[data-field="commit-id"]')!.textContent = summary["Deployment-Commit-Hash"];

    // Counters
    document.querySelector('[data-field="hosts-total"]')!.textContent = summary.Counters.Hosts.toString();
    document.querySelector('[data-field="hosts-completed"]')!.textContent = summary.Counters["Hosts-Completed"].toString();
    document.querySelector('[data-field="hosts-failed"]')!.textContent = summary.Counters["Hosts-Failed"].toString();

    document.querySelector('[data-field="items-total"]')!.textContent = summary.Counters.Items.toString();
    document.querySelector('[data-field="items-completed"]')!.textContent = summary.Counters["Items-Completed"].toString();
    document.querySelector('[data-field="items-failed"]')!.textContent = summary.Counters["Items-Failed"].toString();

    // Highlight status visually
    const statusBox = document.getElementById("summary-status")!;
    statusBox.classList.remove("success", "error");

    if (summary.Status.toLowerCase() === "deployed" || summary.Status.toLowerCase() === "success") {
        statusBox.classList.add("success");
    } else {
        statusBox.classList.add("error");
    }

    // Fill in host details
    const hostList = document.getElementById("host-list")!;
    hostList.innerHTML = "";

    if (summary.Hosts?.length) {
        for (const host of summary.Hosts) {
            const el = document.createElement("details");
            el.className = "host-summary";

            const statusIcon = host.Status === "Deployed" ? "✓" : "✗";
            const statusClass = host.Status === "Deployed" ? "Deployed" : "Failed";

            el.innerHTML = `
                <summary>
                    ${host.Name}
                    <span class="status ${statusClass}">${statusIcon}</span>
                </summary>
                <div class="host-meta">
                    ${host["Error-Message"] ? `<div><strong>Error:</strong> ${host["Error-Message"]}</div>` : ""}
                    ${host["Total-Items"] ? `<div><strong>Items:</strong> ${host["Total-Items"]}</div>` : ""}
                    ${host["Transferred-Size"] ? `<div><strong>Transferred:</strong> ${host["Transferred-Size"]}</div>` : ""}
                </div>
                ${host.Items?.length
                    ? `<ul class="item-list">
                            ${host.Items.map(
                        item => `
                                    <li>
                                        <strong>${item.Name}</strong> —
                                        <span class="action">${item["Deployment-Action"]}</span>
                                        ${item.Status ? `(<span class="status-text">${item.Status}</span>)` : ""}
                                        ${item["Error-Message"] ? `<br><span class="error-text">${item["Error-Message"]}</span>` : ""}
                                    </li>
                                `
                    ).join("")}
                        </ul>`
                    : ""
                }
            `;
            hostList.appendChild(el);
        }
    }

    // Unhide the summary panel
    summaryEl.classList.remove("hidden");
}

window.addEventListener("DOMContentLoaded", () => {
    initRepoDropdown();

    const params = new URLSearchParams(window.location.search);

    // Auto-fill the commit ID input
    const commitID = params.get("commitid");
    if (commitID) {
        const inputEl = document.getElementById("commitid") as HTMLInputElement | null;
        if (inputEl) {
            inputEl.value = commitID;
        }
    }

    // Enable auto-rollback toggle if present
    const autorollbackParam = params.get("autorollback");
    if (autorollbackParam === "true") {
        const checkbox = document.getElementById("opt-autorollback") as HTMLInputElement | null;
        if (checkbox) {
            checkbox.checked = true;
        }
    }

    setupOverrideHostsDropdown();

    document.querySelector(".deploy-final-btn")?.addEventListener("click", handleDeployClick);
});
