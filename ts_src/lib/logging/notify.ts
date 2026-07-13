import { getElement } from "../dom/lookup.js"

// Defer UI initialization until DOM is ready to avoid null document.body
if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", initWarnErrUI);
} else {
    initWarnErrUI();
}

export function initWarnErrUI() {
    // Popup modal
    if (!document.getElementById("warnerr-modal")) {
        const modal = document.createElement("div");
        modal.id = "warnerr-modal";
        modal.className = "modal hidden";
        modal.innerHTML = `
      <div class="modal-content">
        <p id="warnerr-modal-text"></p>
        <button id="warnerr-modal-ok">OK</button>
      </div>`;
        document.body.appendChild(modal);
    }

    // Notification container
    if (!document.getElementById("notification-container")) {
        const notificationContainer = document.createElement("div");
        notificationContainer.id = "notification-container";
        notificationContainer.className = "notification-container";
        document.body.appendChild(notificationContainer);
    }
}

// Error Popup with required exit button
export function showConfirmPopup(message: string) {
    const modal = getElement("warnerr-modal")
    const text = getElement("warnerr-modal-text")
    const okBtn = getElement("warnerr-modal-ok")

    text.textContent = message
    modal.classList.remove("hidden")

    okBtn.onclick = () => {
        modal.classList.add("hidden")
    }
}

// Generic notification renderer used by both warnings and errors
export function showNotification(message: string, className: string, durationMs: number) {
    const container = getElement("notification-container")
    const notification = document.createElement("div")
    notification.className = `notification ${className}`
    notification.textContent = message
    container.appendChild(notification)

    setTimeout(() => {
        notification.remove()
    }, durationMs)
}
