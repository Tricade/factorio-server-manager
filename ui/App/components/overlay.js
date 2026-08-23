let bodyScrollLocks = 0;
let originalBodyOverflow = "";

export const lockBodyScroll = () => {
    if (bodyScrollLocks === 0) originalBodyOverflow = document.body.style.overflow;
    bodyScrollLocks += 1;
    document.body.style.overflow = "hidden";

    return () => {
        bodyScrollLocks = Math.max(0, bodyScrollLocks - 1);
        if (bodyScrollLocks === 0) document.body.style.overflow = originalBodyOverflow;
    };
};

const focusableSelector = [
    "a[href]",
    "button:not([disabled])",
    "input:not([disabled])",
    "select:not([disabled])",
    "textarea:not([disabled])",
    "[tabindex]:not([tabindex='-1'])"
].join(",");

export const focusableElements = root => root
    ? [...root.querySelectorAll(focusableSelector)].filter(element => !element.hidden && element.getAttribute("aria-hidden") !== "true")
    : [];

export const trapFocus = (event, root) => {
    if (event.key !== "Tab") return;
    const elements = focusableElements(root);
    if (elements.length === 0) {
        event.preventDefault();
        root?.focus();
        return;
    }

    const first = elements[0];
    const last = elements[elements.length - 1];
    if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
    }
};
