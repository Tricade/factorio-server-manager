const test = require("node:test");
const assert = require("node:assert/strict");
const http = require("node:http");
const {createRequire} = require("node:module");

// Check the actual transitive dependency resolved by Axios, with synthetic data
// sent only between disposable loopback servers.
const redirects = createRequire(require.resolve("axios"))("follow-redirects");

async function listen(t, handler) {
    const server = http.createServer(handler);
    t.after(async () => {
        server.closeAllConnections();
        await new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve()));
    });
    await new Promise((resolve, reject) => {
        server.once("error", reject);
        server.listen(0, "127.0.0.1", resolve);
    });
    return `http://127.0.0.1:${server.address().port}`;
}

async function redirectFixture(t) {
    const echo = (request, response) => {
        response.setHeader("Content-Type", "application/json");
        response.end(JSON.stringify(request.headers));
    };
    const otherOrigin = await listen(t, echo);
    return listen(t, (request, response) => {
        if (request.url === "/echo") return echo(request, response);
        response.writeHead(302, {Location: request.url === "/same" ? "/echo" : `${otherOrigin}/echo`});
        response.end();
    });
}

function requestHeaders(url, options) {
    return new Promise((resolve, reject) => {
        const request = redirects.http.get(url, {...options, maxRedirects: 2}, response => {
            let body = "";
            response.setEncoding("utf8");
            response.on("data", chunk => { body += chunk; });
            response.on("error", reject);
            response.on("end", () => {
                try {
                    assert.equal(response.statusCode, 200);
                    resolve(JSON.parse(body));
                } catch (error) { reject(error); }
            });
        });
        request.on("error", reject);
        request.setTimeout(5000, () => request.destroy(new Error("Loopback redirect timed out")));
    });
}

test("Axios redirect dependency strips array-form credentials across origins", {timeout: 15000}, async t => {
    // GHSA-q3jh-wrc6-jgq8: numeric array indexes used to bypass header stripping.
    const base = await redirectFixture(t);
    const options = {headers: ["Host", new URL(base).host, "Authorization", "Bearer synthetic", "Cookie", "synthetic=1", "X-Public", "keep"]};
    const same = await requestHeaders(`${base}/same`, options);
    assert.equal(same.authorization, "Bearer synthetic");
    assert.equal(same.cookie, "synthetic=1");

    const cross = await requestHeaders(`${base}/cross`, options);
    assert.equal(cross.authorization, undefined);
    assert.equal(cross.cookie, undefined);
    assert.equal(cross["x-public"], "keep");
});

test("Axios redirect dependency treats sensitive header names literally", {timeout: 15000}, async t => {
    // GHSA-8r9p-f939-6h3c: valid ^ and | characters must not become regex syntax.
    const base = await redirectFixture(t);
    const options = {
        headers: {"X-Api^Key": "synthetic-key", "X-Token|Secret": "synthetic-token", "X-Public": "keep"},
        sensitiveHeaders: ["X-Api^Key", "X-Token|Secret"]
    };
    const same = await requestHeaders(`${base}/same`, options);
    assert.equal(same["x-api^key"], "synthetic-key");
    assert.equal(same["x-token|secret"], "synthetic-token");

    const cross = await requestHeaders(`${base}/cross`, options);
    assert.equal(cross["x-api^key"], undefined);
    assert.equal(cross["x-token|secret"], undefined);
    assert.equal(cross["x-public"], "keep");
});
