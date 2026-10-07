const selectedOptionalNames = plan => [...new Set((plan?.optional || []).filter(item => item.selected).map(item => item.name))];
const canMergePack = (plan, busy, locked) => Boolean(plan?.revision && plan?.additions?.length && !plan?.conflicts?.length && !busy && !locked);
module.exports = {selectedOptionalNames, canMergePack};
