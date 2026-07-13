import type { Result, ElementId } from "../result.js"

// Gets an element by ID - throws when the element is not found in the DOM.
export function getElement(id: string): HTMLElement {
    var el = document.getElementById(id)
    if (el != null) {
        return el
    }
    throw new Error(`getElement: element '${id}' not found`)
}

// Wraps document.getElementById into a Result to avoid ! assertions
export function mustElement<S extends Element>(elId: ElementId): Result<S> {
    var result: Result<S>
    var el = document.getElementById(elId)
    if (el == null) {
        result = { ok: false, error: `element '${elId}' not found` }
        return result
    }
    result = { ok: true, value: el as unknown as S }
    return result
}

// Wraps document.querySelector into a Result to avoid ! assertions
export function mustQuerySelector<S extends Element>(selector: string): Result<S> {
    var result: Result<S>
    var el = document.querySelector<S>(selector)
    if (el == null) {
        result = { ok: false, error: `querySelector '${selector}' returned nil` }
        return result
    }
    result = { ok: true, value: el }
    return result
}
