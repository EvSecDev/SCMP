
// Creates a <span> for file status badges used in repo and dir views
export function createStatusSpan(status: string): HTMLSpanElement {
    const span = document.createElement("span")
    span.className = `file-status status-${status.toLowerCase()}`
    span.textContent = status
    return span
}

export interface PaginationState {
    current: number;  // current page (1-based) or offset
    pageSize: number; // items per page
    total: number;    // total items
}

export function createPagination(
    getTotalItems: () => number,
    pageSizeEl: HTMLSelectElement,
    prevBtn: HTMLButtonElement,
    nextBtn: HTMLButtonElement,
    pageInfoEl: HTMLSpanElement,
    onPage: (page: number, size: number) => void
): { getPage: () => number; getPageSize: () => number; setPage: (p: number) => void; setPageSize: (s: number) => void; refresh: () => void } {
    let pageSize = parseInt(pageSizeEl.value, 10);
    let currentPage = 1;

    const getTotalPages = (): number => {
        let max = Math.ceil(getTotalItems() / pageSize)
        if (max < 1) {
            max = 1
        }
        return max
    }
    const getCurrent = (): number => {
        if (currentPage > getTotalPages()) {
            return getTotalPages()
        }
        return currentPage
    }

    const update = () => {
        const totalPages = getTotalPages();
        const current = getCurrent();
        prevBtn.disabled = current <= 1;
        nextBtn.disabled = current >= totalPages;
        pageInfoEl.textContent = `Page ${current} of ${totalPages}`;
    };

    const handlePageChange = (newPage: number, newSize: number) => {
        onPage(newPage, newSize);
        update();
    };

    pageSizeEl.addEventListener("change", () => {
        pageSize = parseInt(pageSizeEl.value, 10);
        currentPage = 1;
        handlePageChange(getCurrent(), pageSize);
    });

    prevBtn.addEventListener("click", () => {
        if (getCurrent() > 1) {
            currentPage = getCurrent() - 1;
            handlePageChange(getCurrent(), pageSize);
        }
    });

    nextBtn.addEventListener("click", () => {
        if (getCurrent() < getTotalPages()) {
            currentPage = getCurrent() + 1;
            handlePageChange(getCurrent(), pageSize);
        }
    });

    update();

    return {
        getPage: () => getCurrent(),
        getPageSize: () => pageSize,
        setPage: (p: number) => {
            currentPage = p;
            handlePageChange(getCurrent(), pageSize);
        },
        setPageSize: (s: number) => {
            pageSize = s;
            currentPage = 1;
            handlePageChange(getCurrent(), pageSize);
        },
        refresh: () => update(),
    };
}