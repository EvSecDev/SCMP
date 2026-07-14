import { isErr } from "./lib/result.js"
import { getElement } from "./lib/dom/lookup.js"
import { wireSearchInput } from "./lib/dom/filter.js"
import { callRPC } from "./lib/rpc/client.js"
import { logError } from "./lib/logging/log.js"
import { initPage } from "./lib/init/page.js"

interface ApiEntry {
    name: string;
    description: string;
    method: string;
    params: any;
    result: any;
}

function escapeHtml(s: string): string {
    return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;")
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
    var root: TreeNode = { name: "", children: new Map(), expanded: true }

    for (var apiIndex = 0; apiIndex < apis.length; apiIndex++) {
        var api = apis[apiIndex]
        var apiPath = (api as any).path
        var parts = apiPath.replace(/^\/+/, "").split("/")
        var current = root

        for (var partIndex = 0; partIndex < parts.length; partIndex++) {
            var part = parts[partIndex]
            if (!current.children.has(part)) {
                var node: TreeNode = {
                    name: part,
                    children: new Map(),
                    expanded: false,
                    parent: current,
                }
                current.children.set(part, node)
            }
            var childNode = current.children.get(part)
            if (childNode == null) {
                continue
            }
            current = childNode

            if (partIndex === parts.length - 1) {
                current.fullPath = apiPath
            }
        }
    }

    return root
}

function renderTree(
    container: HTMLElement,
    detailsEl: HTMLElement,
    filter: string = ""
) {
    container.innerHTML = "";

    function createNodeElement(node: TreeNode): HTMLElement | null {
        var fullName = node.fullPath
        if (!fullName) {
            fullName = getFullPath(node)
        }
        var matchesFilter = !filter || fullName.toLowerCase().includes(filter.toLowerCase())

        var visible = matchesFilter
        if (!visible) {
            visible = hasMatchingDescendants(node, filter)
        }
        if (!visible) return null;

        var li = document.createElement("li")
        var row = document.createElement("div")
        row.className = "tree-row"

        var hasChildren = node.children.size > 0
        var isLeaf = !!node.fullPath

        if (hasChildren) {
            var toggle = document.createElement("span")
            if (node.expanded) {
                toggle.textContent = "▼"
            } else {
                toggle.textContent = "▶"
            }
            toggle.className = "tree-toggle";
            toggle.onclick = (e) => {
                e.stopPropagation();
                node.expanded = !node.expanded;
                renderTree(container, detailsEl, filter);
            };
            row.appendChild(toggle);
        } else {
            var spacer = document.createElement("span")
            spacer.textContent = "  "
            spacer.className = "tree-spacer"
            row.appendChild(spacer)
        }

        var label = document.createElement("span")
        label.textContent = node.name
        if (isLeaf) {
            label.className = "tree-leaf"
        } else {
            label.className = "tree-branch"
        }
        row.appendChild(label);

        if (isLeaf && node.fullPath) {
            row.className += " tree-leaf-row"
            row.onclick = () => {
                var apiPath = node.fullPath
                if (apiPath == null) {
                    apiPath = getFullPath(node)
                }
                var api = apiMap.get(apiPath)
                if (api) {
                    showDetails(api, detailsEl)
                }
            }
        } else if (hasChildren) {
            row.className += " tree-branch-row"
            row.onclick = () => {
                node.expanded = !node.expanded
                renderTree(container, detailsEl, filter)
            }
        }

        li.appendChild(row);

        if (hasChildren && node.expanded) {
            var ul = document.createElement("ul")
            var childKeys = Array.from(node.children.keys())
            for (var childKeyIndex = 0; childKeyIndex < childKeys.length; childKeyIndex++) {
                const ck = childKeys[childKeyIndex]
                if (ck == null) {
                    continue
                }
                var child = node.children.get(ck)
                if (child == null) {
                    continue
                }
                var childEl = createNodeElement(child)
                if (childEl) {
                    ul.appendChild(childEl)
                }
            }
            li.appendChild(ul)
        }

        return li
    }

    var ul = document.createElement("ul")
    var rootKeys = Array.from(treeRoot.children.keys())
    for (var rootKeyIndex = 0; rootKeyIndex < rootKeys.length; rootKeyIndex++) {
        const rk = rootKeys[rootKeyIndex]
        if (rk == null) {
            continue
        }
        var child = treeRoot.children.get(rk)
        if (child == null) {
            continue
        }
        var el = createNodeElement(child)
        if (el) {
            ul.appendChild(el)
        }
    }

    container.appendChild(ul)
}

function getFullPath(node: TreeNode): string {
    const parts: string[] = [];
    let current: TreeNode | undefined = node;
    while (current && current.name) {
        parts.unshift(current.name);
        current = current.parent;
    }
    return "/" + parts.join("/");
}

function hasMatchingDescendants(node: TreeNode, filter: string): boolean {
    var result: boolean
    var childValues = Array.from(node.children.values())
    for (var childIndex = 0; childIndex < childValues.length; childIndex++) {
        const childItem = childValues[childIndex]
        if (childItem == null) {
            continue
        }
        const child = childItem;
        var fullPath = child.fullPath
        if (!fullPath) {
            fullPath = getFullPath(child)
        }
        if (fullPath.toLowerCase().includes(filter.toLowerCase())) {
            result = true
            return result
        }
        if (hasMatchingDescendants(child, filter)) {
            result = true
            return result
        }
    }
    result = false
    return result
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
    let html = `<h2>${escapeHtml(api.method)}</h2>`;
    if (api.description) {
        html += `<p><em>${escapeHtml(api.description)}</em></p>`;
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

    details.innerHTML = html

    // Resize text areas dynamically
    var textareas = details.querySelectorAll("textarea")
    for (var textareaIndex = 0; textareaIndex < textareas.length; textareaIndex++) {
        const textarea = textareas[textareaIndex] as HTMLTextAreaElement;

        const resize = () => {
            textarea.style.height = "auto"
            textarea.style.height = `${Math.min(textarea.scrollHeight, window.innerHeight * 0.6)}px`
        }

        resize()
        textarea.addEventListener("input", resize)
    }
}


async function loadAPIs() {
    initPage();

    const result = await callRPC<null, ApiEntry[]>("api.browser");
    if (isErr(result)) {
        logError(`apibrowser: load APIs: ${result.error}`, false)
        return;
    }

    var apiEntries: ApiEntry[] = result.value

    // Build internal structures
    for (var entryIndex = 0; entryIndex < apiEntries.length; entryIndex++) {
        const entryItem = apiEntries[entryIndex]
        if (entryItem == null) {
            continue
        }
        const entry = entryItem;
        var methodStr: string
        if (entry.method) {
            methodStr = entry.method
        } else {
            methodStr = ""
        }
        var methodSlash: string
        if (methodStr.length > 0) {
            methodSlash = methodStr.split(".").join("/")
        } else {
            methodSlash = ""
        }
        var entryPath = `/${methodSlash}`;
        (entry as any).path = entryPath;
        apiMap.set(entryPath, entry);
    }

    treeRoot = buildPathTree(Array.from(apiMap.values()))

    var listEl = getElement("api-list")
    var detailsEl = getElement("api-details")
    var searchInput = getElement("api-search") as HTMLInputElement

    wireSearchInput(searchInput, (query) => {
        renderTree(listEl, detailsEl, query)
    })

    renderTree(listEl as HTMLElement, detailsEl as HTMLElement)
}

document.addEventListener("DOMContentLoaded", loadAPIs);
