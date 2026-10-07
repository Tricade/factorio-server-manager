const test = require('node:test');
const assert = require('node:assert/strict');
const {selectedOptionalNames, canMergePack} = require('../App/views/Mods/components/modPackMerge.cjs');

test('optional defaults use server-reviewed selections and can remain empty', () => {
    assert.deepEqual(selectedOptionalNames(null), []);
    assert.deepEqual(selectedOptionalNames({optional: [{name: 'a', selected: false}]}), []);
    assert.deepEqual(selectedOptionalNames({optional: [
        {name: 'a', selected: true}, {name: 'b', selected: false}, {name: 'a', selected: true}, {name: 'c', selected: true}
    ]}), ['a', 'c']);
});

test('adding a pack requires a current nonempty conflict-free preview and stopped writable profile', () => {
    const valid = {revision: 'abc', additions: [{name: 'a'}], conflicts: []};
    assert.equal(canMergePack(valid, false, false), true);
    for (const plan of [null, {}, {...valid, revision: ''}, {...valid, additions: []}, {...valid, conflicts: [{name: 'a'}]}]) {
        assert.equal(canMergePack(plan, false, false), false);
    }
    assert.equal(canMergePack(valid, true, false), false);
    assert.equal(canMergePack(valid, false, true), false);
});
