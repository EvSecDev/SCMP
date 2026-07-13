// Wires a search input element to a filter callback, handling input normalization.
export function wireSearchInput(
    input: HTMLInputElement,
    onFilter: (query: string) => void,
    options?: { enterKey?: boolean }
): void {
    input.addEventListener("input", () => {
        var query = input.value.trim()
        onFilter(query)
    })

    if (options && options.enterKey) {
        input.addEventListener("keydown", (e) => {
            if (e.key === "Enter") {
                e.preventDefault()
                var query = input.value.trim()
                onFilter(query)
            }
        })
    }
}

// Filters a list of elements by their text content, showing/hiding via display style.
export function filterElementsByText(
    elements: HTMLElement[],
    query: string
): void {
    var lowerQuery = query.toLowerCase()
    for (var i = 0; i < elements.length; i++) {
        const row = elements[i];
        if (row == null) continue;
        var text = row.textContent
        if (text == null) {
            text = ""
        }
        if (text.toLowerCase().includes(lowerQuery)) {
            row.style.display = "block"
        } else {
            row.style.display = "none"
        }
    }
}
