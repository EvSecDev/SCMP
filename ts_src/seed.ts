import { initPage } from "./lib/init/page.js"
import { setupOverrideHostsDropdown } from "./ui/dropdown.js"

window.addEventListener("DOMContentLoaded", () => {
    initPage();
    setupOverrideHostsDropdown();
});
