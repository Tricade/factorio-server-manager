const defaultMessage = "The request could not be completed.";

const directText = value => {
    if (typeof value === "string") return value.trim() || null;
    if (!value || typeof value !== "object") return null;
    if (typeof value.error === "string" && value.error.trim()) return value.error.trim();
    if (typeof value.message === "string" && value.message.trim()) return value.message.trim();
    return null;
};

const fallbackText = fallback => directText(fallback) || defaultMessage;

const plainTextMessage = (value, fallback = defaultMessage) =>
    directText(value) || fallbackText(fallback);

const errorMessage = (error, fallback = defaultMessage) =>
    directText(error?.response?.data) ||
    directText(error?.userMessage) ||
    directText(error?.message) ||
    fallbackText(fallback);

module.exports = {plainTextMessage, errorMessage};
