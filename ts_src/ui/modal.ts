import { getElement } from "../lib/dom/lookup.js"
import { readCheckboxValue, readInputValue, readSelectValue } from "../lib/dom/form.js"
import { logAlert } from "../lib/logging/log.js"

export interface ModalOptions {
    message: string;
    inputPlaceholder?: string;
    confirmText?: string;
    cancelText?: string;
    checkboxes?: { id: string; label: string; default?: boolean }[];
    selects?: { id: string; label: string; options: string[]; default?: string }[];
    inputs?: { id: string; label: string; placeholder?: string; default?: string }[];
    hideInput?: boolean;
    onConfirm: (
        inputValue?: string,
        checkboxValues?: Record<string, boolean>,
        selectValues?: Record<string, string>,
        inputValues?: Record<string, string>
    ) => boolean | void | Promise<boolean | void>;
    onCanceled?: () => void;
}

// Cache loaded template
let modalLoaded = false;

let modalLoadPromise: Promise<void> | null = null;

async function ensureModalLoaded(): Promise<void> {
    if (modalLoaded) return
    if (modalLoadPromise) return modalLoadPromise

    modalLoadPromise = (async () => {
        const resp = await fetch("/templates/modal.html");
        if (!resp.ok) {
            throw new Error(`Failed to load modal template: HTTP ${resp.status}`);
        }
        const html = await resp.text();
        const div = document.createElement("div");
        div.innerHTML = html;
        document.body.appendChild(div);
        modalLoaded = true;
    })();

    await modalLoadPromise;
}

export async function showModal(options: ModalOptions) {
    await ensureModalLoaded()

    var modal = getElement("modal")
    var textElem = getElement("modal-text")
    var inputElem = getElement("modal-input") as HTMLInputElement
    var checkboxContainer = getElement("modal-checkbox-container")
    var inputContainer = getElement("modal-input-container")
    var selectContainer = getElement("modal-select-container")
    var confirmBtn = getElement("modal-confirm")
    var cancelBtn = getElement("modal-cancel")

    // Message
    textElem.textContent = options.message;

    // Input
    if (options.inputPlaceholder !== undefined) {
        inputElem.classList.remove("hidden");
        inputElem.placeholder = options.inputPlaceholder;
        inputElem.value = "";

        // Set input type based on hideInput option
        if (options.hideInput) {
            inputElem.type = "password"
        } else {
            inputElem.type = "text"
        }
    } else {
        inputElem.classList.add("hidden");
    }

    // Render extra inputs
    if (options.inputs && options.inputs.length > 0) {
        inputContainer.classList.remove("hidden");
        inputContainer.innerHTML = "";

        for (var inputIndex = 0; inputIndex < options.inputs.length; inputIndex++) {
            const inpItem = options.inputs[inputIndex]
            if (inpItem == null) {
                continue
            }
            const inp = inpItem;
            var wrapper = document.createElement("div")
            wrapper.classList.add("modal-extra-input-wrapper")

            var input = document.createElement("input")
            input.type = "text"
            input.id = inp.id
            if (inp.default) {
                input.value = inp.default
            } else {
                input.value = ""
            }
            if (inp.placeholder) {
                input.placeholder = inp.placeholder
            }
            input.classList.add("modal-extra-input")

            var inputLabelText = document.createElement("span")
            inputLabelText.textContent = inp.label
            inputLabelText.classList.add("modal-extra-input-label")

            wrapper.appendChild(input)
            wrapper.appendChild(inputLabelText)
            inputContainer.appendChild(wrapper)
        }
    } else {
        inputContainer.classList.add("hidden");
    }

    // Check boxes
    if (options.checkboxes && options.checkboxes.length > 0) {
        checkboxContainer.classList.remove("hidden");
        checkboxContainer.innerHTML = ""; // clear old ones
        for (var checkboxIndex = 0; checkboxIndex < options.checkboxes.length; checkboxIndex++) {
            const cbItem = options.checkboxes[checkboxIndex]
            if (cbItem == null) {
                continue
            }
            const cb = cbItem;
            var checkboxLabel = document.createElement("label")
            var checkbox = document.createElement("input")
            checkbox.type = "checkbox"
            checkbox.id = cb.id
            if (cb.default) {
                checkbox.checked = cb.default
            } else {
                checkbox.checked = false
            }
            checkboxLabel.appendChild(checkbox)
            checkboxLabel.append(` ${cb.label}`)
            checkboxContainer.appendChild(checkboxLabel)
        }
    } else {
        checkboxContainer.classList.add("hidden");
    }

    // Render selects
    if (options.selects && options.selects.length > 0) {
        selectContainer.classList.remove("hidden");
        selectContainer.innerHTML = ""; // clear old ones

        for (var selectIndex = 0; selectIndex < options.selects.length; selectIndex++) {
            const selItem = options.selects[selectIndex]
            if (selItem == null) {
                continue
            }
            const sel = selItem;
            var label = document.createElement("label")
            label.textContent = sel.label

            var select = document.createElement("select")
            select.id = sel.id

            for (var optionIndex = 0; optionIndex < sel.options.length; optionIndex++) {
                const optItem = sel.options[optionIndex]
                if (optItem == null) {
                    continue
                }
                const opt = optItem;
                var optionElem = document.createElement("option")
                optionElem.value = opt
                optionElem.textContent = opt
                if (sel.default === opt) {
                    optionElem.selected = true
                }
                select.appendChild(optionElem)
            }

            label.appendChild(select)
            selectContainer.appendChild(label)
        }
    } else {
        selectContainer.classList.add("hidden");
    }

    // Buttons
    // Remove old handlers before showing modal
    confirmBtn.onclick = null;
    cancelBtn.onclick = null;

    if (options.confirmText) {
        confirmBtn.textContent = options.confirmText
    } else {
        confirmBtn.textContent = "Confirm"
    }

    if (options.cancelText !== undefined && options.cancelText !== null) {
        cancelBtn.classList.remove("hidden");
        cancelBtn.textContent = options.cancelText;
        confirmBtn.classList.remove("single-confirm");
    } else {
        cancelBtn.classList.add("hidden");
        confirmBtn.classList.add("single-confirm");
    }

    // Remove old keyboard handler if present, then attach new one
    modal.removeEventListener("keydown", modalKeydownHandler)
    modal.addEventListener("keydown", modalKeydownHandler as EventListener)

    var handleConfirm = () => {
        var inputValue: string | undefined
        if (inputElem && !inputElem.classList.contains("hidden")) {
            inputValue = inputElem.value.trim()
        }

        var inputValues: Record<string, string> = {}
        if (options.inputs) {
            for (var inputIndex = 0; inputIndex < options.inputs.length; inputIndex++) {
                const inpItem = options.inputs[inputIndex]
                if (inpItem == null) {
                    continue
                }
                const inp = inpItem;
                inputValues[inp.id] = readInputValue(inp.id)
            }
        }

        var checkboxValues: Record<string, boolean> = {}
        if (options.checkboxes) {
            for (var checkboxIndex = 0; checkboxIndex < options.checkboxes.length; checkboxIndex++) {
                const cbItem = options.checkboxes[checkboxIndex]
                if (cbItem == null) {
                    continue
                }
                const cb = cbItem;
                checkboxValues[cb.id] = readCheckboxValue(cb.id)
            }
        }

        var selectValues: Record<string, string> = {}
        if (options.selects) {
            for (var selectIndex = 0; selectIndex < options.selects.length; selectIndex++) {
                const selItem = options.selects[selectIndex]
                if (selItem == null) {
                    continue
                }
                const sel = selItem;
                selectValues[sel.id] = readSelectValue(sel.id)
            }
        }

        // Validation if needed
        if (inputElem && !inputElem.classList.contains("hidden") && !inputValue) {
            logAlert("Please enter the required value to proceed.");
            inputElem.focus();
            return;
        }

        const result = options.onConfirm(inputValue, checkboxValues, selectValues, inputValues);
        const closeModal = () => { modal.classList.add("hidden"); };

        if (result instanceof Promise) {
            result.then(cancelled => {
                if (cancelled !== false) {
                    closeModal();
                }
            }).catch(() => {
                closeModal();
            });
        } else if (result !== false) {
            closeModal();
        }
    };

    var handleCancel = () => {
        if (options.onCanceled) {
            options.onCanceled();
        }
        modal.classList.add("hidden");
    };

    confirmBtn.onclick = handleConfirm;
    cancelBtn.onclick = handleCancel;

    modal.classList.remove("hidden");

    if (inputElem && !inputElem.classList.contains("hidden")) {
        inputElem.focus();
    }
}

function modalKeydownHandler(e: KeyboardEvent) {
    if (e.key === "Enter") {
        e.preventDefault();
        var confirmBtn = document.getElementById("modal-confirm")
        if (confirmBtn) {
            confirmBtn.click();
        }
    } else if (e.key === "Escape") {
        e.preventDefault();
        var cancelBtn = document.getElementById("modal-cancel")
        if (cancelBtn && !cancelBtn.classList.contains("hidden")) {
            cancelBtn.click();
        }
    }
}
