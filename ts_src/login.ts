let usernameInput: HTMLInputElement | null = null
let passwordInput: HTMLInputElement | null = null
let loginBtn: HTMLElement | null = null

function showPopup() {
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

function initLogin() {
    const usernameInputRaw = document.getElementById("username")
    const passwordInputRaw = document.getElementById("password")
    const loginBtnRaw = document.getElementById("loginBtn")

    if (
        !(usernameInputRaw instanceof HTMLInputElement) ||
        !(passwordInputRaw instanceof HTMLInputElement) ||
        loginBtnRaw == null
    ) {
        throw new Error("Required DOM elements not found or have wrong types")
    }

    usernameInput = usernameInputRaw
    passwordInput = passwordInputRaw
    loginBtn = loginBtnRaw

    loginBtn.addEventListener("click", () => {
        handleLogin()
    })

    for (let i = 0; i < [usernameInput, passwordInput].length; i++) {
        const input = [usernameInput, passwordInput][i]
        if (input == null) {
            continue
        }
        input.addEventListener("keydown", (event) => {
            if (event.key === "Enter") {
                event.preventDefault()
                handleLogin()
            }
        })
    }
}

async function handleLogin() {
    if (!usernameInput || !passwordInput) {
        return
    }

    const username = usernameInput.value.trim()
    const password = passwordInput.value.trim()

    if (!username || !password) {
        console.error("Username and password are required")
        showPopup()
        return
    }

    const rpcRequest = {
        jsonrpc: "2.0",
        method: "user.login",
        params: { username, password },
        id: "1",
    }

    try {
        const response = await fetch("/api/", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
                Accept: "application/json",
            },
            body: JSON.stringify(rpcRequest),
            credentials: "same-origin",
        })

        if (!response.ok) {
            console.error("Login failed with HTTP status:", response.status)
            showPopup()
            return
        }

        const data = await response.json()

        if (data.error) {
            const code = data.error.code
            if (code === -32001 || code === -32002) {
                showPopup()
                return
            }
            console.error("RPC Error:", data.error)
            showPopup()
            return
        }

        const authToken = data.result
        if (!authToken || !authToken.id_token) {
            console.error("Invalid login response")
            showPopup()
            return
        }

        var maxAgeSeconds = 0
        if (authToken.validTime) {
            maxAgeSeconds = authToken.validTime
        }
        document.cookie = `id_token=${encodeURIComponent(authToken.id_token)}; path=/; max-age=${maxAgeSeconds}`

        const redirectUrl = authToken.redirectTo || "/index.html"
        window.location.href = redirectUrl
    } catch (err) {
        console.error("Network or other error:", err)
        showPopup()
    }
}

if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", initLogin)
} else {
    initLogin()
}
