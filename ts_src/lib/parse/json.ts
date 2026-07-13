import type { Result } from "../result.js"

const rpcErrorMessages: Record<number, string> = {
    // Standard JSON-RPC 2 codes
    [-32700]: "Parse Error",
    [-32600]: "Invalid Request",
    [-32601]: "Method Not Found",
    [-32602]: "Invalid Params",
    [-32603]: "Internal Error",

    // Custom error codes
    [-32001]: "Unauthorized",
    [-32003]: "Conflict",
    [-32012]: "Invalid State",
};

// Wraps JSON.parse into a Result
export function parseJSON<T>(text: string): Result<T> {
    var result: Result<T>
    var parsedValue: T

    if (text === "") {
        result = { ok: false, error: "empty string" }
        return result
    }

    try {
        parsedValue = JSON.parse(text)
        result = { ok: true, value: parsedValue }
        return result
    } catch (err: unknown) {
        var errMsg = "unknown error"
        if (err instanceof Error) {
            errMsg = err.message
        }
        result = { ok: false, error: `parse JSON: ${errMsg}` }
        return result
    }
}

// Strips JSONRPC wrapper to return either error message or result object
export function parseApiResponse<T = any>(data: unknown): Result<T> {
    var result: Result<T>

    if (!data || typeof data !== "object" || Array.isArray(data)) {
        result = { ok: false, error: "response missing jsonrpc field" }
        return result
    }

    const obj = data as Record<string, unknown>
    if (!hasKey(obj, "jsonrpc")) {
        result = { ok: false, error: "response missing jsonrpc field" }
        return result
    }
    if (hasKey(obj, "result")) {
        const version = getStringField(obj, "jsonrpc")
        if (version != "2.0") {
            result = { ok: false, error: "invalid JSON-RPC version" }
            return result
        }
        result = { ok: true, value: getField(obj, "result") as T }
        return result
    }
    if (hasKey(obj, "error")) {
        const version = getStringField(obj, "jsonrpc")
        if (version != "2.0") {
            result = { ok: false, error: "invalid JSON-RPC version" }
            return result
        }
        const errObj = getField(obj, "error")
        const err = errObj as { code?: number; message?: string; data?: string }
        const translatedCode = rpcErrorMessages[err.code as number]
        var translated = ""
        if (translatedCode) {
            translated = translatedCode
        } else {
            var codeStr = 0
            if (err.code != null) {
                codeStr = err.code
            }
            translated = `UnknownCode(${codeStr})`
        }
        var msg = ""
        if (err.message) {
            msg = err.message
        }
        var errorText = `${translated}: ${msg}`
        if (err.data) {
            errorText += `: ${err.data}`
        }
        return { ok: false, error: errorText }
    }

    result = { ok: false, error: "response missing 'result' and 'error'" }
    return result
}

// Checks if a value has a given string key (avoids 'as' cast)
function hasKey(obj: unknown, key: string): boolean {
    var result: boolean
    if (typeof obj !== "object" || obj === null) {
        result = false
        return result
    }
    var recordObj = obj as Record<string, unknown>
    result = typeof recordObj[key] !== "undefined"
    return result
}

// Safely gets a field from an object
function getField(obj: Record<string, unknown>, key: string): unknown {
    var result: unknown
    result = obj[key]
    return result
}

// Safely gets a string field from an object
function getStringField(obj: Record<string, unknown>, key: string): string {
    var result: string
    var val = obj[key]
    if (typeof val === "string") {
        result = val
        return result
    }
    result = ""
    return result
}
