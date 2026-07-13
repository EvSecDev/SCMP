import type { Result } from "../result.js"

// Wraps atob into a Result
export function safeAtob(text: string): Result<string> {
    var result: Result<string>
    if (text === "") {
        result = { ok: false, error: "empty string" }
        return result
    }

    try {
        var decoded = atob(text)
        result = { ok: true, value: decoded }
        return result
    } catch (err: unknown) {
        var errMsg = "unknown error"
        if (err instanceof Error) {
            errMsg = err.message
        }
        result = { ok: false, error: `atob: ${errMsg}` }
        return result
    }
}

// Wraps TextEncoder into a Result
export function doTextEncode(text: string): Result<Uint8Array<ArrayBuffer>> {
    var result: Result<Uint8Array<ArrayBuffer>>
    try {
        var encoder = new TextEncoder()
        var bytes = encoder.encode(text)
        result = { ok: true, value: bytes }
        return result
    } catch (err: unknown) {
        var errMsg = "unknown error"
        if (err instanceof Error) {
            errMsg = err.message
        }
        result = { ok: false, error: `text encoder: ${errMsg}` }
        return result
    }
}
