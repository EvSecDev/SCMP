function loadSidebar(): void {
    fetch("/templates/sidebar.html")
        .then((response) => {
            if (!response.ok) {
                throw new Error(`Failed to load sidebar: HTTP ${response.status}`)
            }
            return response.text()
        })
        .then((html) => {
            const placeholder = document.getElementById("sidebar-placeholder")
            if (placeholder) {
                placeholder.outerHTML = html
            }
        })
        .catch((error) => {
            console.error("Sidebar load error:", error)
        })
}

loadSidebar()

