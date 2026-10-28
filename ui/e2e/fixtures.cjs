const {test: base, expect} = require("@playwright/test");

const test = base.extend({
    // A hosted runner can take almost the entire test budget to create its
    // first WebKit page, before any application URL is requested. Keep that
    // cold-start budget separate from the unchanged 30-second behavior tests.
    // https://playwright.dev/docs/test-fixtures#fixture-timeout
    page: [async ({context}, use) => {
        const page = await context.newPage();
        try {
            await use(page);
        } finally {
            await page.close();
        }
    }, {timeout: 60000}]
});

module.exports = {test, expect};
