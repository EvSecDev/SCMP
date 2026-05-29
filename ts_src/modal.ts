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
    ) => void;
}

// Cache loaded template
let modalLoaded = false;

async function ensureModalLoaded(): Promise<void> {
    if (modalLoaded) return;

    const resp = await fetch("/templates/modal.html");
    const html = await resp.text();
    const div = document.createElement("div");
    div.innerHTML = html;
    document.body.appendChild(div);

    modalLoaded = true;
}

export async function showModal(options: ModalOptions) {
    await ensureModalLoaded(); // make sure modal HTML is in DOM

    const modal = document.getElementById("modal")!;
    const textElem = document.getElementById("modal-text") as HTMLParagraphElement;
    const inputElem = document.getElementById("modal-input") as HTMLInputElement;
    const checkboxContainer = document.getElementById("modal-checkbox-container")!;
    const inputContainer = document.getElementById("modal-input-container")!;
    const selectContainer = document.getElementById("modal-select-container")!;
    const confirmBtn = document.getElementById("modal-confirm") as HTMLButtonElement;
    const cancelBtn = document.getElementById("modal-cancel") as HTMLButtonElement;

    // Message
    textElem.textContent = options.message;

    // Input
    if (options.inputPlaceholder !== undefined) {
        inputElem.classList.remove("hidden");
        inputElem.placeholder = options.inputPlaceholder;
        inputElem.value = "";

        // Set input type based on hideInput option
        inputElem.type = options.hideInput ? "password" : "text";
    } else {
        inputElem.classList.add("hidden");
    }

    // Render extra inputs
    if (options.inputs && options.inputs.length > 0) {
        inputContainer.classList.remove("hidden");
        inputContainer.innerHTML = "";

        options.inputs.forEach(inp => {
            const wrapper = document.createElement("div");
            wrapper.classList.add("modal-extra-input-wrapper");

            const input = document.createElement("input");
            input.type = "text";
            input.id = inp.id;
            input.value = inp.default ?? "";
            input.placeholder = inp.placeholder ?? "";
            input.classList.add("modal-extra-input");

            const label = document.createElement("span");
            label.textContent = inp.label;
            label.classList.add("modal-extra-input-label");

            wrapper.appendChild(input);
            wrapper.appendChild(label);
            inputContainer.appendChild(wrapper);
        });
    } else {
        inputContainer.classList.add("hidden");
    }

    // Check boxes
    if (options.checkboxes && options.checkboxes.length > 0) {
        checkboxContainer.classList.remove("hidden");
        checkboxContainer.innerHTML = ""; // clear old ones
        options.checkboxes.forEach(cb => {
            const label = document.createElement("label");
            const checkbox = document.createElement("input");
            checkbox.type = "checkbox";
            checkbox.id = cb.id;
            checkbox.checked = cb.default ?? false;
            label.appendChild(checkbox);
            label.append(" " + cb.label);
            checkboxContainer.appendChild(label);
        });
    } else {
        checkboxContainer.classList.add("hidden");
    }

    // Render selects
    if (options.selects && options.selects.length > 0) {
        selectContainer.classList.remove("hidden");
        selectContainer.innerHTML = ""; // clear old ones

        options.selects.forEach(sel => {
            const label = document.createElement("label");
            label.textContent = sel.label;

            const select = document.createElement("select");
            select.id = sel.id;

            sel.options.forEach(opt => {
                const optionElem = document.createElement("option");
                optionElem.value = opt;
                optionElem.textContent = opt;
                if (sel.default === opt) optionElem.selected = true;
                select.appendChild(optionElem);
            });

            label.appendChild(select);
            selectContainer.appendChild(label);
        });
    } else {
        selectContainer.classList.add("hidden");
    }

    // Buttons
    confirmBtn.textContent = options.confirmText ?? "Confirm";

    if (options.cancelText !== undefined && options.cancelText !== null) {
        cancelBtn.classList.remove("hidden");
        cancelBtn.textContent = options.cancelText;
        cancelBtn.onclick = () => {
            modal.classList.add("hidden");
        };
    } else {
        cancelBtn.classList.add("hidden");
        // center confirm when no cancel
        confirmBtn.classList.add("single-confirm");
    }

    // Remove old handlers
    confirmBtn.onclick = null;
    cancelBtn.onclick = null;

    modal.classList.remove("hidden");

    confirmBtn.onclick = () => {
        const inputValue = inputElem && !inputElem.classList.contains("hidden")
            ? inputElem.value.trim()
            : undefined;

        const inputValues: Record<string, string> = {};
        if (options.inputs) {
            options.inputs.forEach(inp => {
                const el = document.getElementById(inp.id) as HTMLInputElement;
                inputValues[inp.id] = el?.value ?? "";
            });
        }

        const checkboxValues: Record<string, boolean> = {};
        if (options.checkboxes) {
            options.checkboxes.forEach(cb => {
                const el = document.getElementById(cb.id) as HTMLInputElement;
                checkboxValues[cb.id] = el?.checked ?? false;
            });
        }

        const selectValues: Record<string, string> = {};
        if (options.selects) {
            options.selects.forEach(sel => {
                const el = document.getElementById(sel.id) as HTMLSelectElement;
                selectValues[sel.id] = el?.value ?? "";
            });
        }

        // Validation if needed
        if (inputElem && !inputElem.classList.contains("hidden") && !inputValue) {
            alert("Please enter the required value to proceed.");
            inputElem.focus();
            return;
        }

        modal.classList.add("hidden");
        options.onConfirm(inputValue, checkboxValues, selectValues, inputValues);
    };

    cancelBtn.onclick = () => {
        modal.classList.add("hidden");
    };
}
