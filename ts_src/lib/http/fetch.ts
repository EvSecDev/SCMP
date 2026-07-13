import type { Result } from "../result.js"

// Wraps fetch() into a Result
export async function doFetch(url: string, opts: RequestInit): Promise<Result<Response>> {
    var result: Result<Response>
    var mergedOpts: RequestInit = {
        ...opts,
        credentials: "same-origin",
    }
    try {
        var resp = await fetch(url, mergedOpts)
        result = { ok: true, value: resp }
        return result
    } catch (err: unknown) {
        var errMsg = "network error"
        if (err instanceof Error) {
            errMsg = err.message
        }
        result = { ok: false, error: `fetch '${url}': ${errMsg}` }
        return result
    }
}
