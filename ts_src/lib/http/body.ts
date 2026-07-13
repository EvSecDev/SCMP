import type { Result } from "../result.js"

// Wraps response.json() into a Result
export async function readJSON<T>(response: Response): Promise<Result<T>> {
    var result: Result<T>
    try {
        var data = await response.json()
        result = { ok: true, value: data }
        return result
    } catch (err: unknown) {
        var errMsg = "invalid body"
        if (err instanceof Error) {
            errMsg = err.message
        }
        result = { ok: false, error: `response.json: ${errMsg}` }
        return result
    }
}

// Wraps response.text() into a Result
export async function readText(response: Response): Promise<Result<string>> {
    var result: Result<string>
    try {
        var data = await response.text()
        result = { ok: true, value: data }
        return result
    } catch (err: unknown) {
        var errMsg = "invalid body"
        if (err instanceof Error) {
            errMsg = err.message
        }
        result = { ok: false, error: `response.text: ${errMsg}` }
        return result
    }
}
