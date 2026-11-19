import assert from 'node:assert/strict';
import { createHash, generateKeyPairSync, sign, verify } from 'node:crypto';
import { test } from 'node:test';

// Fixture local. La raíz de confianza y el registro de firmas reales son tarea de T007.
const issuer = 'https://token.actions.githubusercontent.com';
const workflow = 'https://github.com/VaivenEnterprise/VaivenApplication-Backend/.github/workflows/deploy-staging.yml@refs/heads/develop';
const trusted = generateKeyPairSync('ed25519');
const other = generateKeyPairSync('ed25519');
const digest = bytes => 'sha256:' + createHash('sha256').update(bytes).digest('hex');
const current = digest('imagen sintética actual');
const previous = digest('imagen sintética anterior');

function attestation(payload, key = trusted.privateKey) {
  const encoded = Buffer.from(JSON.stringify(payload));
  return { encoded, signature: sign(null, encoded, key) };
}

function verifiedDigest(reference, signed) {
  if (!/^sha256:[a-f0-9]{64}$/.test(reference) || !signed) return false;
  if (!verify(null, signed.encoded, trusted.publicKey, signed.signature)) return false;
  const claims = JSON.parse(signed.encoded);
  return claims.issuer === issuer && claims.workflow === workflow && claims.digest === reference;
}

test('firma local y coincidencia de digest para avance y reversión', () => {
  const trustedClaim = { issuer, workflow, digest: current };
  assert.equal(verifiedDigest(current, attestation(trustedClaim)), true);
  assert.equal(verifiedDigest(previous, attestation({ ...trustedClaim, digest: previous })), true);
  assert.equal(verifiedDigest(previous, attestation(trustedClaim)), false);
  assert.equal(verifiedDigest('staging-latest', attestation(trustedClaim)), false);
  assert.equal(verifiedDigest(current, null), false);
  assert.equal(verifiedDigest(current, attestation(trustedClaim, other.privateKey)), false);
  assert.equal(verifiedDigest(current, attestation({ ...trustedClaim, workflow: 'otra-identidad' })), false);
  assert.equal(verifiedDigest(current, attestation({ ...trustedClaim, issuer: 'otro-emisor' })), false);
});

test('el informe sintético de seguridad bloquea un hallazgo alto y rechaza entradas inválidas', () => {
  const passes = findings => Array.isArray(findings) && findings.every(({ severity }) => ['info', 'low', 'medium'].includes(severity));
  assert.equal(passes([{ severity: 'low' }]), true);
  assert.equal(passes([{ severity: 'medium' }, { severity: 'high' }]), false);
  assert.equal(passes([{ severity: 'critical' }]), false);
  assert.equal(passes([{ severity: 'unknown' }]), false);
  assert.equal(passes(null), false);
});

test('casos de aceptación de las seis alertas con entradas sintéticas', () => {
  const scenarios = [
    ['A-1', n => n >= 3, [2, 3]],
    ['A-2', n => n >= 50, [49, 50]],
    ['A-3', (count, requests) => count >= 10 || (requests >= 50 && count / requests > 0.02), [[1, 50], [2, 50]]],
    ['A-4', (count, oomKilled) => count >= 3 || oomKilled, [[2, false], [0, true]]],
    ['A-5', (severity, fixed) => severity === 'critical' && fixed, [['critical', false], ['critical', true]]],
    ['A-6', result => result === 'verification_failed', ['verified', 'verification_failed']],
  ];
  for (const [id, triggers, [within, over]] of scenarios) {
    const args = input => Array.isArray(input) ? input : [input];
    assert.equal(triggers(...args(within)), false, id);
    assert.equal(triggers(...args(over)), true, id);
  }
});
