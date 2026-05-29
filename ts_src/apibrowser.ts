interface ApiEntry {
    name: string;
    description: string;
    method: string;
    params: any;
    result: any;
}

interface TreeNode {
    name: string;
    fullPath?: string;
    children: Map<string, TreeNode>;
    expanded: boolean;
    parent?: TreeNode;
}

// Persistent root
let treeRoot: TreeNode;
const apiMap = new Map<string, ApiEntry>();

function buildPathTree(apis: ApiEntry[]): TreeNode {
    const root: TreeNode = { name: "", children: new Map(), expanded: true };

    for (const api of apis) {
        const parts = (api as any).path.replace(/^\/+/, "").split("/");
        let current = root;

        for (let i = 0; i < parts.length; i++) {
            const part = parts[i];
            if (!current.children.has(part)) {
                const node: TreeNode = {
                    name: part,
                    children: new Map(),
                    expanded: false,
                    parent: current,
                };
                current.children.set(part, node);
            }
            current = current.children.get(part)!;

            if (i === parts.length - 1) {
                current.fullPath = (api as any).path;
            }
        }
    }

    return root;
}

function renderTree(
    node: TreeNode,
    container: HTMLElement,
    detailsEl: HTMLElement,
    filter: string = ""
) {
    container.innerHTML = "";

    function createNodeElement(node: TreeNode): HTMLElement | null {
        const fullName = node.fullPath || getFullPath(node);
        const matchesFilter = !filter || fullName.toLowerCase().includes(filter.toLowerCase());

        const visible = matchesFilter || hasMatchingDescendants(node, filter);
        if (!visible) return null;

        const li = document.createElement("li");
        const row = document.createElement("div");
        row.className = "tree-row";

        const hasChildren = node.children.size > 0;
        const isLeaf = !!node.fullPath;

        if (hasChildren) {
            const toggle = document.createElement("span");
            toggle.textContent = node.expanded ? "▼" : "▶";
            toggle.className = "tree-toggle";
            toggle.onclick = (e) => {
                e.stopPropagation();
                node.expanded = !node.expanded;
                renderTree(treeRoot, container, detailsEl, filter);
            };
            row.appendChild(toggle);
        } else {
            const spacer = document.createElement("span");
            spacer.textContent = "  ";
            spacer.className = "tree-spacer";
            row.appendChild(spacer);
        }

        const label = document.createElement("span");
        label.textContent = node.name;
        label.className = isLeaf ? "tree-leaf" : "tree-branch";
        if (isLeaf && node.fullPath) {
            label.onclick = () => {
                const api = apiMap.get(node.fullPath!);
                if (api) showDetails(api, detailsEl);
            };
        }
        row.appendChild(label);

        li.appendChild(row);

        if (hasChildren && node.expanded) {
            const ul = document.createElement("ul");
            node.children.forEach(child => {
                const childEl = createNodeElement(child);
                if (childEl) ul.appendChild(childEl);
            });
            li.appendChild(ul);
        }

        return li;
    }

    const ul = document.createElement("ul");
    treeRoot.children.forEach(child => {
        const el = createNodeElement(child);
        if (el) ul.appendChild(el);
    });

    container.appendChild(ul);
}

function getFullPath(node: TreeNode): string {
    const parts: string[] = [];
    let current: TreeNode | undefined = node;
    while (current && current.name) {
        parts.unshift(current.name);
        current = current.parent;
    }
    return parts.join(".");
}

function hasMatchingDescendants(node: TreeNode, filter: string): boolean {
    for (const child of node.children.values()) {
        const fullPath = child.fullPath || getFullPath(child);
        if (fullPath.toLowerCase().includes(filter.toLowerCase())) return true;
        if (hasMatchingDescendants(child, filter)) return true;
    }
    return false;
}

function renderBody(body: any, bodyType?: string): string {
    if (!bodyType) return "";

    switch (bodyType.toLowerCase()) {
        case "string":
            return `<pre class="body-placeholder">[ raw string body ]</pre>`;

        case "[]byte":
        case "byte[]":
            return `<pre class="body-placeholder">[ binary/octet-stream ]</pre>`;

        case "json":
        default:
            return `<textarea readonly>${JSON.stringify(body, null, 2)}</textarea>`;
    }
}

function showDetails(api: ApiEntry, details: HTMLElement) {
    let html = `<h2>${api.method}</h2>`;
    if (api.description) {
        html += `<p><em>${api.description}</em></p>`;
    }

    // Request Body
    if (api.params && Object.keys(api.params).length > 0) {
        html += `<h4>Request Body</h4>`;
        html += renderBody(api.params, "json");
    }

    // Response Body
    if (api.result !== null && api.result !== undefined) {
        html += `<h4>Response Body</h4>`;
        html += renderBody(api.result, "json");
    }

    details.innerHTML = html;

    // Resize text areas dynamically
    details.querySelectorAll("textarea").forEach((ta) => {
        const textarea = ta as HTMLTextAreaElement;

        const resize = () => {
            textarea.style.height = "auto";
            textarea.style.height = Math.min(textarea.scrollHeight, window.innerHeight * 0.6) + "px";
        };

        resize(); // adjust immediately for prefilled content
        textarea.addEventListener("input", resize);
    });
}


async function loadAPIs() {
    const rpcRequest = {
        jsonrpc: "2.0",
        method: "api.browser",
        params: {},
        id: "1",
    };

    const headers: Record<string, string> = {
        "Content-Type": "application/json",
    };

    const res = await fetch("/api/", {
        method: "POST",
        headers,
        body: JSON.stringify(rpcRequest),
    });

    if (!res.ok) {
        return { ok: false, error: `HTTP ${res.status}` };
    }

    const json = await res.json();
    const apiEntries: ApiEntry[] = json.result;

    // Build internal structures
    for (const entry of apiEntries) {
        const path = "/" + entry.method.replace(/\./g, "/");
        (entry as any).path = path;
        apiMap.set(path, entry);
    }

    treeRoot = buildPathTree(Array.from(apiMap.values()));

    const list = document.getElementById("api-list")!;
    const details = document.getElementById("api-details")!;
    const searchInput = document.getElementById("api-search") as HTMLInputElement;

    searchInput.addEventListener("input", () => {
        const filter = searchInput.value.trim();
        renderTree(treeRoot, list, details, filter);
    });

    renderTree(treeRoot, list, details);
}

document.addEventListener("DOMContentLoaded", loadAPIs);
