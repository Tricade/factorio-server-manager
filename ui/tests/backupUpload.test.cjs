const {test} = require("node:test");
const assert = require("node:assert/strict");
const {createBackupUploader, backupFileError, backupErrorMessage, formatBackupSize} = require("../api/resources/backupUpload.cjs");

const mib = 1024 ** 2;
const limits = size => ({max_upload_bytes: size * mib, max_file_bytes: size * mib - 68 * 1024});

test("oversized profile and mod backups never start a multipart POST", async () => {
    for (const path of ["/api/profiles/import/preview", "/api/profiles/import", "/api/mods/backup/preview", "/api/mods/backup/restore"]) {
        let posts = 0;
        const uploader = createBackupUploader({get: async () => ({data: limits(512)}), post: async () => { posts++; }});
        await assert.rejects(uploader.upload(path, {size: 556 * mib}), /512 MiB.*FSM_MAX_UPLOAD/);
        assert.equal(posts, 0);
    }
});

test("preflight uses current server limits for every request and preserves form fields", async () => {
    let current = limits(1);
    let gets = 0;
    const posted = [];
    const uploader = createBackupUploader({
        get: async path => { assert.equal(path, "/api/backups/limits"); gets++; return {data: current}; },
        post: async (path, form, options) => { posted.push({path, form, options}); return {data: {ok: true}}; }
    });
    const file = new Blob([new Uint8Array(2 * mib)]);
    await assert.rejects(uploader.upload("/api/profiles/import/preview", file), /1 MiB/);
    current = limits(4);
    assert.deepEqual(await uploader.upload("/api/profiles/import", file, {selection: '[{"id":"a","name":"Restored"}]'}), {ok: true});
    assert.equal(gets, 2);
    assert.equal(posted.length, 1);
    assert.equal(posted[0].form.get("backup").size, file.size);
    assert.equal(posted[0].form.get("backup").name, "backup.zip");
    assert.equal(posted[0].form.get("selection"), '[{"id":"a","name":"Restored"}]');
    assert.match(posted[0].options.networkErrorMessage, /proxy.*temporary disk space/);
});

test("unavailable or malformed limits fail closed without guessing a default", async () => {
    for (const data of [null, {}, {max_upload_bytes: -1, max_file_bytes: 1}, {max_upload_bytes: 12, max_file_bytes: 13}, {max_upload_bytes: Infinity, max_file_bytes: 1}]) {
        const uploader = createBackupUploader({get: async () => ({data}), post: () => assert.fail("must not upload")});
        await assert.rejects(uploader.upload("/api/profiles/import/preview", new Blob(["zip"])), /upload limit.*Retry/i);
    }
    const uploader = createBackupUploader({get: async () => { throw new Error("offline"); }, post: () => assert.fail("must not upload")});
    await assert.rejects(uploader.upload("/api/profiles/import/preview", new Blob(["zip"])), /offline/);
});

test("file checks reserve multipart space and distinguish profile from mod advice", () => {
    assert.equal(backupFileError({size: 365 * mib}, limits(512), "profiles"), "");
    assert.equal(backupFileError({size: limits(512).max_file_bytes}, limits(512), "profiles"), "");
    assert.match(backupFileError({size: limits(512).max_file_bytes + 1}, limits(512), "profiles"), /metadata/);
    assert.match(backupFileError({size: 556 * mib}, limits(512), "profiles"), /one profile at a time/);
    assert.doesNotMatch(backupFileError({size: 556 * mib}, limits(512), "mods"), /one profile/);
    assert.match(backupFileError({size: 0}, limits(512)), /empty/);
    assert.equal(formatBackupSize(512 * mib), "512 MiB");
    assert.equal(formatBackupSize(16 * 1024 * mib), "16 GiB");
    assert.equal(formatBackupSize(1024), "1 KiB");
    assert.equal(formatBackupSize(0), "0 B");
});

test("backup error messages preserve actionable preflight, HTTP and transport errors", () => {
    assert.equal(backupErrorMessage({userMessage: "Upload interrupted: check proxy."}), "Upload interrupted: check proxy.");
    assert.equal(backupErrorMessage({response: {data: "HTTP size limit exceeded"}}), "HTTP size limit exceeded");
    assert.equal(backupErrorMessage(new Error("internal detail")), "The backup could not be processed. Try again.");
});
