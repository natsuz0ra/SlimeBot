import assert from 'node:assert/strict';
import test from 'node:test';
import { formatPrivacyStatus, runPrivacyCommand } from './privacy.js';
import type { PrivacyConfig, PrivacySnapshot } from '../types/privacy.js';

function fixture() {
  const config: PrivacyConfig = {
    enabled: false,
    secrets: true,
    personal: true,
    kinds: { PHONE: false },
    words: ['do-not-print'],
    ignoreCase: false,
    wholeWords: false,
    rules: [],
    allow: ['sensitive-exception'],
    actions: {},
    bypassProviderIds: [],
    modelEnabled: false,
    modelThreshold: 0.8,
    modelTimeoutSeconds: 15,
    mappingPersistence: true,
    mappingTtlMinutes: 1440,
  };
  const snapshot: PrivacySnapshot = { config, model: { installed: false, installing: false, running: false, stage: 'not_installed', downloadedBytes: 0, totalBytes: 39000000 }, records: [] };
  let saved: PrivacyConfig | undefined;
  const destroyedSessions: string[] = [];
  const client = {
    getPrivacy: async () => snapshot,
    updatePrivacy: async (value: PrivacyConfig) => { saved = value; },
    previewPrivacy: async (text: string) => ({ text: `masked: ${text.length}`, hits: [{ kind: 'PASSWORD', action: 'mask' }], blocked: false }),
    clearPrivacyRecords: async () => {},
    destroySessionPrivacyMappings: async (sessionId: string) => { destroyedSessions.push(sessionId); },
    installPrivacyModel: async () => ({}), cancelPrivacyModel: async () => ({}), removePrivacyModel: async () => {},
  };
  return { snapshot, client, getSaved: () => saved, destroyedSessions };
}

test('privacy status omits sensitive configuration values and displays retention policy', () => {
  const value = formatPrivacyStatus(fixture().snapshot);
  assert.match(value, /Local masking: OFF/);
  assert.match(value, /Mappings: encrypted · TTL 1440m/);
  assert.ok(!value.includes('do-not-print'));
  assert.ok(!value.includes('sensitive-exception'));
});

test('privacy destroy calls session mapping purge endpoint', async () => {
  const state = fixture();
  await assert.rejects(runPrivacyCommand(state.client, 'destroy'), /Open or start a session first/);
  const output = await runPrivacyCommand(state.client, 'destroy', 'sess-123');
  assert.deepEqual(state.destroyedSessions, ['sess-123']);
  assert.match(output, /Current session privacy mappings destroyed/);
});

test('privacy config patches retain other category settings', async () => {
  const state = fixture();
  await runPrivacyCommand(state.client, 'config {"kinds":{"EMAIL":true},"actions":{"PASSWORD":"block"}}');
  assert.deepEqual(state.getSaved()?.kinds, { PHONE: false, EMAIL: true });
  assert.equal(state.getSaved()?.actions.PASSWORD, 'block');
  await assert.rejects(runPrivacyCommand(state.client, 'config {"unknown":true}'), /Unknown/);
  await assert.rejects(runPrivacyCommand(state.client, 'config []'), /JSON object/);
});

test('privacy toggle and preview use backend and never persist preview text', async () => {
  const state = fixture();
  await runPrivacyCommand(state.client, 'on');
  assert.equal(state.getSaved()?.enabled, true);
  await runPrivacyCommand(state.client, 'model on');
  assert.equal(state.getSaved()?.modelEnabled, true);
  const output = await runPrivacyCommand(state.client, 'try MySecret123');
  assert.match(output, /1 sensitive/);
  assert.ok(!output.includes('MySecret123'));
  assert.equal(state.getSaved()?.words[0], 'do-not-print');
});
