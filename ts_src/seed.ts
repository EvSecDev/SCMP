import { getJSONViaJSON, logError, isErr, initRepoDropdown, initVersionInfo } from "./helpers.js";
import { setupOverrideHostsDropdown, selectedHosts } from "./dropdown.js";

window.addEventListener("DOMContentLoaded", () => {
    initVersionInfo();
    initRepoDropdown();
    setupOverrideHostsDropdown();
});
