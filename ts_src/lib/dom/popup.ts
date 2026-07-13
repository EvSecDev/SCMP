// Show popup with fade in/out
export function showPopup() {
    const popupRaw = document.getElementById("accessDeniedPopup")
    if (!(popupRaw instanceof HTMLElement)) {
        return
    }

    var popup = popupRaw
    popup.style.display = "block"
    popup.style.opacity = "1"

    setTimeout(() => {
        popup.style.opacity = "0"
        setTimeout(() => {
            popup.style.display = "none"
        }, 500)
    }, 3000)
}
