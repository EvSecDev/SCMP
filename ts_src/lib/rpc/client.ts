import type { Result } from "../result.js"
import { isErr } from "../result.js"
import { ensureRepoHeader, getCurrentRepoHeader, httpAPIPath, repoHeaderKey } from "./auth.js"
import { fetchRPC, parseRpcResponse } from "./transport.js"
import { doTextEncode } from "../parse/encoding.js"
import { doFetch } from "../http/fetch.js"
import { readJSON } from "../http/body.js"

// JSONRPC wrapper and API query entry point
let rpcIdCounter = 0

function nextRpcId(): string {
    rpcIdCounter += 1
    return String(rpcIdCounter)
}

export async function getJSONViaJSON<reqJSON = unknown, respJSON = any>(
    rpcMethod: string,
    payload?: reqJSON
): Promise<Result<respJSON>> {
    var result: Result<respJSON>
    await ensureRepoHeader()

    var jsonPayload: unknown = payload
    if (payload == null) {
        jsonPayload = {}
    }

    var rpcRequest: Record<string, unknown> = {
        jsonrpc: "2.0",
        method: rpcMethod,
        params: jsonPayload,
        id: nextRpcId(),
    }

    var extraHeaders: Record<string, string> = {}
    var repo = getCurrentRepoHeader()
    if (repo) {
        extraHeaders[repoHeaderKey] = repo
    }

    var response = await fetchRPC(httpAPIPath, rpcRequest, extraHeaders)
    if (!response.ok) {
        result = { ok: false, error: response.error }
        return result
    }

    result = parseRpcResponse<respJSON>(response.data, response.status)
    return result
}

// RPC call variant that does NOT set the SCMP-Repository header.
// Used for calls made before repo is selected (api browser).
export async function callRPC<ReqJSON = unknown, RespJSON = any>(
    rpcMethod: string,
    payload?: ReqJSON
): Promise<Result<RespJSON>> {
    var result: Result<RespJSON>
    var jsonPayload: unknown = payload
    if (payload == null) {
        jsonPayload = {}
    }

    var rpcRequest: Record<string, unknown> = {
        jsonrpc: "2.0",
        method: rpcMethod,
        params: jsonPayload,
        id: nextRpcId(),
    }

    var response = await fetchRPC(httpAPIPath, rpcRequest, {})
    if (!response.ok) {
        result = { ok: false, error: response.error }
        return result
    }

    result = parseRpcResponse<RespJSON>(response.data, response.status)
    return result
}

// Sending raw body with an expected JSONRPC response
export async function sendData<RespJSON = any>(
    path: string,
    method: string,
    payload: string,
    expectJsonResponse: boolean = true
): Promise<Result<RespJSON | void>> {
    var result: Result<RespJSON | void>
    var encodeResult = doTextEncode(payload)
    if (isErr(encodeResult)) {
        result = { ok: false, error: `sendData: ${encodeResult.error}` }
        return result
    }
    var bytes = encodeResult.value

    var headers: Record<string, string> = {
        "Content-Type": "application/octet-stream",
    }
    if (expectJsonResponse) {
        headers.Accept = "application/json"
    }

    var fetchResult = await doFetch(path, {
        method: method,
        headers: headers,
        body: bytes,
    })
    if (isErr(fetchResult)) {
        result = { ok: false, error: `sendData: ${fetchResult.error}` }
        return result
    }
    var response = fetchResult.value

    if (!expectJsonResponse) {
        if (!response.ok) {
            result = { ok: false, error: `HTTP ${response.status}` }
            return result
        }
        result = { ok: true, value: undefined }
        return result
    }

    var bodyResult = await readJSON(response)
    if (isErr(bodyResult)) {
        result = { ok: false, error: `HTTP ${response.status}: ${bodyResult.error}` }
        return result
    }

    result = parseRpcResponse<RespJSON>(bodyResult.value, response.status)
    return result
}
