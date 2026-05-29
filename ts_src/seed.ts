import { getJSONViaJSON, logError, isErr, initRepoDropdown } from "./helpers.js";
import { setupOverrideHostsDropdown, selectedHosts } from "./dropdown.js";

window.addEventListener("DOMContentLoaded", () => {
    initRepoDropdown();
    setupOverrideHostsDropdown();
});
