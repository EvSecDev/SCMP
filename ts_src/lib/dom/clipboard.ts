import type { Result } from "../result.js"

// Wraps navigator.clipboard.writeText into a Result
export async function safeCopyToClipboard(text: string): Promise<Result<void>> {
    var result: Result<void>
    try {
        await navigator.clipboard.writeText(text)
        result = { ok: true, value: undefined }
        return result
    } catch (err: unknown) {
        var errMsg = "clipboard copy failed"
        if (err instanceof Error) {
            errMsg = err.message
        }
        result = { ok: false, error: `copyToClipboard: ${errMsg}` }
        return result
    }
}
