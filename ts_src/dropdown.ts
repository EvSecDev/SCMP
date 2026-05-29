import { getJSONViaJSON } from "./helpers.js";

export let selectedHosts: Set<string> = new Set();

export async function setupOverrideHostsDropdown() {
    const inputEl = document.getElementById("overridehosts-input") as HTMLInputElement;
    const dropdownEl = document.getElementById("overridehosts-dropdown") as HTMLDivElement;

    const result = await getJSONViaJSON("settings.hosts.list", { withDetails: false });

    dropdownEl.innerHTML = "";
    if (!result.ok) {
        dropdownEl.innerHTML = `<div class="dropdown-loading">Error loading hosts</div>`;
        return;
    }

    const hosts: string[] = result.value?.hosts ?? [];
    if (hosts.length === 0) {
        dropdownEl.innerHTML = `<div class="dropdown-loading">No hosts available</div>`;
        return;
    }

    // --- SEARCH ROW ---
    const searchRow = document.createElement("div");
    searchRow.className = "host-row search-row";
    const searchInput = document.createElement("input");
    searchInput.type = "text";
    searchInput.placeholder = "Search...";
    searchRow.appendChild(searchInput);
    dropdownEl.appendChild(searchRow);

    // --- HOST ROWS ---
    const rowElements: HTMLDivElement[] = [];
    hosts.forEach(host => {
        const row = document.createElement("div");
        row.className = "host-row";
        row.textContent = host;

        row.addEventListener("click", () => {
            if (selectedHosts.has(host)) {
                selectedHosts.delete(host);
                row.classList.remove("selected");
            } else {
                selectedHosts.add(host);
                row.classList.add("selected");
            }
            updateInputValue();
        });

        dropdownEl.appendChild(row);
        rowElements.push(row);
    });

    function updateInputValue() {
        inputEl.value = Array.from(selectedHosts).join(", ");
    }

    // --- FILTER HOST ROWS ---
    searchInput.addEventListener("input", () => {
        const query = searchInput.value.toLowerCase();
        rowElements.forEach(row => {
            const text = row.textContent?.toLowerCase() ?? "";
            row.style.display = text.includes(query) ? "block" : "none";
        });
    });

    // --- DROPDOWN TOGGLE ---
    dropdownEl.classList.add("hidden");

    inputEl.addEventListener("click", (e) => {
        e.stopPropagation();
        dropdownEl.classList.toggle("hidden");

        if (!dropdownEl.classList.contains("hidden")) {
            searchInput.focus();
            searchInput.select();
        }
    });

    dropdownEl.addEventListener("click", (e) => {
        e.stopPropagation();
    });

    document.addEventListener("click", () => {
        dropdownEl.classList.add("hidden");
    });
}
