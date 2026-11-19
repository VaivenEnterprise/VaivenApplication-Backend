import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const [template, parameters] = process.argv.slice(2).map(path => JSON.parse(readFileSync(path, 'utf8')));
const gate = "[variables('pilotEligible')]";

function verifyPilot(arm, defaults) {
  assert.deepEqual(defaults.parameters, {
    environment: { value: 'staging' },
    enablePilot: { value: false },
    inventoryAndBudgetConfirmed: { value: false },
    pilot: { value: null },
  });
  for (const [name, parameter] of Object.entries(defaults.parameters).filter(([name]) => name !== 'pilot')) {
    assert.deepEqual(arm.parameters[name].defaultValue, parameter.value, `${name} template default`);
  }
  assert.equal(arm.variables.pilotEligible, "[and(and(and(equals(parameters('environment'), 'staging'), parameters('enablePilot')), parameters('inventoryAndBudgetConfirmed')), variables('configurationReady'))]");
  const { vnet, subnet, pilotNetwork, pilotVm, pilotLoadBalancer } = arm.resources;
  assert.deepEqual(Object.keys(arm.resources).sort(), ['pilotLoadBalancer', 'pilotNetwork', 'pilotVm', 'subnet', 'vnet']);
  assert.equal(vnet.type, 'Microsoft.Network/virtualNetworks');
  assert.equal(subnet.type, 'Microsoft.Network/virtualNetworks/subnets');
  assert.ok(vnet.existing && subnet.existing, 'VNet and subnet must remain existing references');
  for (const module of [pilotNetwork, pilotVm, pilotLoadBalancer]) {
    assert.equal(module.type, 'Microsoft.Resources/deployments');
    assert.equal(module.condition, gate);
    assert.equal(module.properties.mode, 'Incremental');
  }

  const network = pilotNetwork.properties.template;
  const [nic, nsg] = Object.values(network.resources).sort((left, right) => left.type.localeCompare(right.type));
  assert.equal(Object.values(network.resources).length, 2);
  assert.equal(nic.type, 'Microsoft.Network/networkInterfaces');
  assert.equal(nsg.type, 'Microsoft.Network/networkSecurityGroups');
  assert.equal(nic.properties.enableIPForwarding, false);
  assert.equal(nic.properties.ipConfigurations.length, 1);
  assert.equal(nic.properties.ipConfigurations[0].properties.privateIPAllocationMethod, 'Static');
  assert.match(nic.properties.networkSecurityGroup.id, /networkSecurityGroups/);
  assert.equal(nsg.properties.securityRules, "[concat(variables('baseRules'), variables('adminRules'), variables('relayRules'))]");
  const rules = network.variables.baseRules.map(({ properties: rule }) => [rule.direction, rule.access, rule.sourceAddressPrefix, rule.destinationAddressPrefix, rule.destinationPortRange]);
  const privateIp = "[parameters('pilotVmPrivateIp')]";
  assert.deepEqual(rules, [
    ['Inbound', 'Allow', "[parameters('clientSourceCidr')]", privateIp, '8443'],
    ['Inbound', 'Allow', 'AzureLoadBalancer', privateIp, '8443'],
    ['Inbound', 'Deny', '*', '*', '*'],
    // Spec 018: VM2 solo llega a la pasarela mTLS de VM1, nunca a las bases.
    ['Outbound', 'Allow', privateIp, "[parameters('mongoPrivateIp')]", "[string(parameters('mongoGatewayPort'))]"],
    ['Outbound', 'Allow', privateIp, "[parameters('redisPrivateIp')]", "[string(parameters('redisGatewayPort'))]"],
    ['Outbound', 'Allow', privateIp, 'AzurePlatformDNS', '53'],
    ['Outbound', 'Deny', '*', '*', '*'],
  ]);
  assert.match(network.variables.adminRules, /parameters\('adminSourceCidr'\).+'destinationPortRange', '22'/);
  assert.match(network.variables.relayRules, /parameters\('artifactRelayPrivateIp'\).+'destinationPortRange', string\(parameters\('relayPort'\)\)/);
  assert.equal(network.parameters.mongoGatewayPort.defaultValue, 27018);
  assert.equal(network.parameters.redisGatewayPort.defaultValue, 6380);
  assert.equal(network.parameters.relayPort.defaultValue, 8444);

  const [vm] = Object.values(pilotVm.properties.template.resources);
  assert.equal(Object.values(pilotVm.properties.template.resources).length, 1);
  assert.equal(vm.type, 'Microsoft.Compute/virtualMachines');
  const { osProfile, storageProfile, securityProfile, networkProfile } = vm.properties;
  assert.equal(osProfile.linuxConfiguration.disablePasswordAuthentication, true);
  assert.equal(osProfile.linuxConfiguration.ssh.publicKeys.length, 1);
  assert.equal(osProfile.adminPassword, undefined);
  assert.equal(storageProfile.osDisk.createOption, 'FromImage');
  assert.equal(storageProfile.osDisk.deleteOption, 'Delete');
  assert.equal(networkProfile.networkInterfaces.length, 1);
  assert.equal(networkProfile.networkInterfaces[0].properties.deleteOption, 'Delete');
  assert.equal(networkProfile.networkInterfaces[0].id, "[parameters('nicId')]");
  assert.deepEqual(securityProfile, { securityType: 'TrustedLaunch', uefiSettings: { secureBootEnabled: true, vTpmEnabled: true } });

  const [balancer] = Object.values(pilotLoadBalancer.properties.template.resources);
  assert.equal(Object.values(pilotLoadBalancer.properties.template.resources).length, 1);
  assert.equal(balancer.type, 'Microsoft.Network/loadBalancers');
  assert.equal(balancer.sku.name, 'Standard');
  const { frontendIPConfigurations, backendAddressPools, probes, loadBalancingRules, outboundRules } = balancer.properties;
  assert.equal(frontendIPConfigurations.length, 1);
  assert.equal(frontendIPConfigurations[0].properties.subnet.id, "[parameters('subnetId')]");
  assert.equal(backendAddressPools.length, 1);
  assert.deepEqual(pilotLoadBalancer.properties.parameters.settings.value.backendPrivateIps, ["[variables('existingVmPrivateIp')]", "[variables('pilotVmPrivateIp')]"]);
  assert.equal(backendAddressPools[0].properties.copy[0].count, "[length(parameters('settings').backendPrivateIps)]");
  assert.equal(pilotLoadBalancer.properties.parameters.tlsPort.value, 8443);
  assert.equal(probes.length, 1);
  assert.equal(probes[0].properties.protocol, 'Https');
  assert.equal(probes[0].properties.requestPath, '/healthz');
  assert.equal(loadBalancingRules.length, 1);
  assert.equal(loadBalancingRules[0].properties.protocol, 'Tcp');
  assert.equal(loadBalancingRules[0].properties.probe.id, "[variables('probeId')]");
  assert.equal(outboundRules, undefined);
  assert.doesNotMatch(JSON.stringify(arm), /publicIPAddress/i);
}

verifyPilot(template, parameters);
const regressions = [
  arm => { arm.resources.pilotVm.condition = true; },
  arm => { arm.resources.vnet.existing = false; },
  arm => { Object.values(arm.resources.pilotNetwork.properties.template.resources)[1].properties.ipConfigurations[0].properties.publicIPAddress = {}; },
  arm => { arm.resources.pilotLoadBalancer.properties.parameters.settings.value.backendPrivateIps.pop(); },
  arm => { arm.resources.pilotNetwork.properties.template.variables.baseRules.pop(); },
];
for (const mutate of regressions) {
  const candidate = structuredClone(template);
  mutate(candidate);
  assert.throws(() => verifyPilot(candidate, parameters), assert.AssertionError);
}
console.log(`Pilot ARM validated, including ${regressions.length} rejected regressions.`);
