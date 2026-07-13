import type { Result } from "../result.js"
import { showNotification, showConfirmPopup } from "./notify.js"

// Entrance point for all warnings
export function logWarning(message: string) {
    console.warn(message)
    showNotification(message, "warning", 15000)
}

// Entrance point for all errors
export function logError(message: string, popupConfirm: boolean) {
    console.error(message)

    if (popupConfirm) {
        showConfirmPopup(message)
    } else {
        showNotification(message, "", 15000)
    }
}

// User-facing alert for validation errors - logs + shows confirm popup + returns error Result
export function logAlert(message: string): Result<void> {
    console.error(message)
    showNotification(message, "", 8000)
    return { ok: false, error: message }
}