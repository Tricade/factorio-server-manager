const refreshAfterMutation = async refresh => {
    try {
        await refresh();
        return true;
    } catch {
        return false;
    }
};

const refreshFailureMessage = "The change was saved, but the latest mod data could not be loaded. Use Retry to refresh it.";

module.exports = {refreshAfterMutation, refreshFailureMessage};
