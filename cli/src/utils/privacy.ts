import type { PrivacyConfig, PrivacyPreview, PrivacySnapshot } from '../types/privacy.js';

interface PrivacyClient {
  getPrivacy(): Promise<PrivacySnapshot>;
  updatePrivacy(config: PrivacyConfig): Promise<void>;
  previewPrivacy(text: string): Promise<PrivacyPreview>;
  clearPrivacyRecords(): Promise<void>;
  installPrivacyModel(): Promise<unknown>;
  cancelPrivacyModel(): Promise<unknown>;
  removePrivacyModel(): Promise<void>;
  destroySessionPrivacyMappings?: (sessionId: string) => Promise<void>;
}

export const privacyHelp = [
  '/privacy on|off',
  '/privacy secrets|personal on|off',
  '/privacy model on|off|install|cancel|remove',
  '/privacy try <text>',
  '/privacy records|clear',
  '/privacy destroy  (destroy current session privacy mappings)',
  '/privacy config <JSON patch>  (rules, words, allow, kinds, actions, bypassProviderIds, threshold, mappingPersistence, mappingTtlMinutes)',
  'Example: /privacy config {"words":["Project-A"],"actions":{"PASSWORD":"block"}}',
].join('\n');

export function formatPrivacyStatus(snapshot: PrivacySnapshot): string {
  const { config, model, mappings } = snapshot;
  const retentionLine = mappings || config.mappingPersistence !== undefined
    ? `Mappings: ${mappings?.mode || (config.mappingPersistence ? 'encrypted' : 'memory')}${config.mappingTtlMinutes ? ` · TTL ${config.mappingTtlMinutes}m` : ''}${mappings?.keyStatus ? ` · key ${mappings.keyStatus}` : ''}`
    : '';
  return [
    `Local masking: ${config.enabled ? 'ON' : 'OFF'} | credentials: ${config.secrets ? 'ON' : 'OFF'} | personal data: ${config.personal ? 'ON' : 'OFF'}`,
    `Local model: ${config.modelEnabled ? 'ON (strict)' : 'OFF'} | ${model.installing ? `installing (${model.stage})` : model.installed ? 'installed' : 'not installed'}`,
    `Custom words: ${config.words.length} | rules: ${config.rules.length} | exceptions: ${config.allow.length} | bypassed providers: ${config.bypassProviderIds.length}`,
    retentionLine,
    model.error ? `Model: ${model.error}` : '',
    '', privacyHelp,
  ].filter(value => value !== '').join('\n');
}

export async function runPrivacyCommand(client: PrivacyClient, raw: string, sessionId = ''): Promise<string> {
  const command = raw.trim();
  if (command === 'destroy') {
    if (!sessionId) throw new Error('Open or start a session first before destroying session privacy mappings.');
    if (!client.destroySessionPrivacyMappings) throw new Error('Session privacy mapping destruction is unavailable.');
    await client.destroySessionPrivacyMappings(sessionId);
    return 'Current session privacy mappings destroyed.';
  }
  if (command.startsWith('try ')) {
    const preview = await client.previewPrivacy(command.slice(4));
    return [`${preview.blocked ? 'BLOCKED' : 'Preview'}: ${preview.hits.length} sensitive occurrences`, preview.text, 'Preview stays local and does not change conversation mappings.'].join('\n');
  }
  if (command === 'clear') { await client.clearPrivacyRecords(); return 'Privacy records cleared.'; }
  if (command === 'model install') { await client.installPrivacyModel(); return 'Local model installation started. Use /privacy to check progress.'; }
  if (command === 'model cancel') { await client.cancelPrivacyModel(); return 'Local model installation cancellation requested.'; }
  if (command === 'model remove') { await client.removePrivacyModel(); return 'Local model removed and disabled.'; }
  const snapshot = await client.getPrivacy();
  if (!command) return formatPrivacyStatus(snapshot);
  if (command === 'records') {
    return snapshot.records.length ? snapshot.records.slice(0, 20).map(record => `${record.time} | ${record.source || 'LLM'} | ${record.blocked ? 'BLOCKED' : record.failed ? 'FAILED' : `${record.masked} masked, ${record.warned} warned`} | ${record.durationMs} ms`).join('\n') : 'No privacy records in this run.';
  }
  let config = snapshot.config;
  if (command === 'on' || command === 'off') config = { ...config, enabled: command === 'on' };
  else if (/^(secrets|personal|model) (on|off)$/.test(command)) {
    const [group, state] = command.split(' ');
    config = { ...config, [group === 'model' ? 'modelEnabled' : group!]: state === 'on' };
  } else if (command.startsWith('config ')) {
    const patch: unknown = JSON.parse(command.slice(7));
    if (!patch || typeof patch !== 'object' || Array.isArray(patch)) throw new Error('Privacy config must be a JSON object.');
    const fields = new Set(Object.keys(config));
    for (const key of Object.keys(patch)) if (!fields.has(key)) throw new Error(`Unknown privacy setting: ${key}`);
    const update = patch as Partial<PrivacyConfig>;
    config = { ...config, ...update, kinds: { ...config.kinds, ...update.kinds }, actions: { ...config.actions, ...update.actions } };
  } else return privacyHelp;
  await client.updatePrivacy(config);
  return 'Privacy settings saved. Changes apply to new model requests.';
}
