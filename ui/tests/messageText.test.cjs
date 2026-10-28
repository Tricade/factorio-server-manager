const {test} = require("node:test");
const assert = require("node:assert/strict");
const {plainTextMessage, errorMessage} = require("../api/messageText.cjs");

test("plain text messages accept strings and only direct string error/message fields", () => {
    assert.equal(plainTextMessage("  readable  "), "readable");
    assert.equal(plainTextMessage({error: "  rejected  "}), "rejected");
    assert.equal(plainTextMessage({message: "try again"}), "try again");
    assert.equal(plainTextMessage({error: {details: "private"}, message: ["invalid"], details: "secret"}), "The request could not be completed.");
    assert.equal(plainTextMessage({toString: () => "do not stringify"}), "The request could not be completed.");
    assert.equal(plainTextMessage({error: {}}, {message: ["bad fallback"]}), "The request could not be completed.");
});

test("API errors use safe response text, then user-facing text, then a supplied fallback", () => {
    assert.equal(errorMessage({response: {data: "  too large  "}}, "fallback"), "too large");
    assert.equal(errorMessage({response: {data: {error: "rejected"}}}, "fallback"), "rejected");
    assert.equal(errorMessage({response: {data: {message: "try again"}}}, "fallback"), "try again");
    assert.equal(errorMessage({response: {data: {error: {details: "secret"}}}, message: "Network error"}, "fallback"), "Network error");
    assert.equal(errorMessage({response: {data: {details: "secret"}}}, "safe fallback"), "safe fallback");
});
