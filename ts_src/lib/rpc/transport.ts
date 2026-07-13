import { isErr } from "../result.js"
import type { Result } from "../result.js"
import { doFetch } from "../http/fetch.js"
import { readJSON } from "../http/body.js"
import { parseApiResponse } from "../parse/json.js"

type RawRPCResponse = {
    ok: boolean
    data: unknown
    status: number
    error: string
}

export async function fetchRPC(url: string, rpcRequest: object, extraHeaders: Record<string, string>): Promise<RawRPCResponse> {
    var result: RawRPCResponse
    var respResult = await doFetch(url, {
        method: "POST",
        headers: buildHeaders(extraHeaders),
        body: JSON.stringify(rpcRequest),
    })
    if (isErr(respResult)) {
        result = { ok: false, data: null, status: 0, error: respResult.error }
        return result
    }

    var bodyResult = await readJSON(respResult.value)
    if (isErr(bodyResult)) {
        result = { ok: false, data: null, status: respResult.value.status, error: bodyResult.error }
        return result
    }

    result = { ok: true, data: bodyResult.value, status: respResult.value.status, error: "" }
    return result
}

// Centralized response parser - applies JSON-RPC parsing + HTTP status prefix
export function parseRpcResponse<T>(rawJSON: unknown, httpStatus: number): Result<T> {
    var result: Result<T>
    if (!rawJSON) {
        var msg = ""
        if (!httpStatus) {
            msg = "no response body"
        } else {
            msg = `HTTP ${httpStatus}`
        }
        result = { ok: false, error: msg }
        return result
    }

    var parsed = parseApiResponse<T>(rawJSON)
    if (!isErr(parsed)) {
        result = parsed
        return result
    }

    if (httpStatus >= 400) {
        result = { ok: false, error: `HTTP ${httpStatus}: ${parsed.error}` }
        return result
    }

    result = parsed
    return result
}


// Centralized fetch wrapper - handles only network I/O, returns RawRPCResponse
function buildHeaders(extraHeaders: Record<string, string>): Record<string, string> {
    var result: Record<string, string> = {
        "Content-Type": "application/json",
        "Accept": "application/json",
    }
    var entries = Object.entries(extraHeaders)
    for (var entryIndex = 0; entryIndex < entries.length; entryIndex++) {
        const entry = entries[entryIndex];
        if (entry == null) continue;
        var headerKey = entry[0]
        var headerValue = entry[1]
        result[headerKey] = headerValue
    }
    return result
}