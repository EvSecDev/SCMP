
// Helper to quickly check error presence
export function isErr<T>(result: Result<T>): result is { ok: false; error: string } {
    return !result.ok;
}

// Result type for explicit value/error handling
export type Result<T> =
    | { ok: true; value: T }
    | { ok: false; error: string };


// Branded type to distinguish element IDs from CSS selectors at compile time
export type ElementId = string & { readonly __brand: unique symbol }

// Wraps a string to create an ElementId for use with mustElement
export function id(s: string): ElementId {
    return s as unknown as ElementId
}
