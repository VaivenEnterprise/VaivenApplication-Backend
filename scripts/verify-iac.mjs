import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

function validate(template, staging, production) {
  const isManaged = ([, resource]) => resource.existing !== true;
  const managed = Object.entries(template.resources).filter(isManaged);
  assert.deepEqual(managed.map(([name]) => name), ['proposedLoadBalancer']);
  const candidate = managed[0][1];
  assert.equal(candidate.type, 'Microsoft.Resources/deployments');
  assert.equal(candidate.condition, "[variables('loadBalancerEligible')]");
  const guard = template.variables.loadBalancerEligible;
  assert.match(guard, /equals\(parameters\('environment'\), 'staging'\)/);
  assert.match(guard, /not\(equals\(parameters\('loadBalancer'\), null\(\)\)\)/);
  assert.match(guard, /variables\('subnetKnown'\)/);
  assert.match(guard, /variables\('hasDistinctBackends'\)/);
  const backends = template.variables.hasDistinctBackends;
  assert.match(backends, /equals\(length\(union\(variables\('backendIps'\), variables\('backendIps'\)\)\), length\(variables\('backendIps'\)\)\)/);
  assert.match(backends, /greaterOrEquals\(length\(variables\('backendIps'\)\), 2\)/);

  for (const [environment, file] of Object.entries({ staging, production })) {
    assert.equal(file.parameters.environment.value, environment);
    assert.equal(file.parameters.loadBalancer.value, null);
  }

  const nested = Object.entries(candidate.properties.template.resources).filter(isManaged);
  assert.deepEqual(nested.map(([, resource]) => resource.type), ['Microsoft.Network/loadBalancers']);
  const balancer = nested[0][1];
  assert.equal(balancer.sku.name, 'Standard');
  const { frontendIPConfigurations, probes, loadBalancingRules, outboundRules } = balancer.properties;
  assert.equal(frontendIPConfigurations.length, 1);
  const frontend = frontendIPConfigurations[0].properties;
  assert.ok(frontend.subnet.id);
  assert.ok(frontend.privateIPAddress);
  assert.equal(frontend.publicIPAddress, undefined);
  assert.equal(outboundRules, undefined);
  assert.equal(probes.length, 1);
  assert.equal(probes[0].properties.protocol, 'Https');
  assert.equal(probes[0].properties.requestPath, '/healthz');
  assert.equal(loadBalancingRules.length, 1);
  const rule = loadBalancingRules[0].properties;
  assert.equal(rule.protocol, 'Tcp');
  assert.equal(rule.frontendPort, rule.backendPort);
  assert.equal(rule.frontendPort, "[parameters('tlsPort')]");
  assert.equal(rule.disableOutboundSnat, true);
}

const [, , mainPath, stagingPath, productionPath] = process.argv;
assert.ok(mainPath && stagingPath && productionPath, 'Indica el ARM compilado y ambos parámetros JSON');
const read = path => JSON.parse(readFileSync(path, 'utf8').replace(/^\uFEFF/, ''));
const template = read(mainPath);
const staging = read(stagingPath);
const production = read(productionPath);
validate(template, staging, production);

// Contrapruebas sobre copias en memoria, sin ARM ni credenciales.
const unexpectedVm = structuredClone(template);
unexpectedVm.resources.injected = { type: 'Microsoft.Compute/virtualMachines' };
assert.throws(() => validate(unexpectedVm, staging, production));
const alwaysOn = structuredClone(template);
delete alwaysOn.resources.proposedLoadBalancer.condition;
assert.throws(() => validate(alwaysOn, staging, production));
const productionOptIn = structuredClone(production);
productionOptIn.parameters.loadBalancer.value = {};
assert.throws(() => validate(template, staging, productionOptIn));
const oneBackend = structuredClone(template);
oneBackend.variables.hasDistinctBackends = "[greaterOrEquals(length(variables('backendIps')), 1)]";
assert.throws(() => validate(oneBackend, staging, production));
console.log('ARM: inventario existente, LB privado optativo y cuatro contrapruebas en verde.');
