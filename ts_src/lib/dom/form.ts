// Reads values from form field elements by ID.
// Returns safe defaults when the element is missing or has wrong type.

function readElementValue(id: string, defaultValue: string): string {
    var result = defaultValue
    var el = document.getElementById(id)
    if (el instanceof HTMLInputElement) {
        result = el.value
    } else if (el instanceof HTMLSelectElement) {
        result = el.value
    }
    return result
}

export function readCheckboxValue(id: string): boolean {
    var el = document.getElementById(id)
    if (el instanceof HTMLInputElement) {
        return el.checked
    }
    return false
}

export function readInputValue(id: string): string {
    return readElementValue(id, "")
}

export function readSelectValue(id: string): string {
    return readElementValue(id, "")
}

export function readRadioGroupValue(name: string): string | null {
    var el = document.querySelector<HTMLInputElement>("input[name=" + name + "]:checked")
    if (el) {
        return el.value
    }
    return null
}

export function parseIntOrZero(value: string): number {
    var parsed = parseInt(value, 10)
    if (Number.isNaN(parsed)) {
        return 0
    }
    return parsed
}
