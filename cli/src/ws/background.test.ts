import assert from 'node:assert/strict';
import test from 'node:test';
import { dispatchWSMessage, type WSHandlers } from './socket';
import { runCliCommand, type CliCommandHandlers } from '../controllers/commands';

test('CLI background notifications stay inside the watched session', () => {
  const events: unknown[] = [];
  const handlers: WSHandlers = {
    acceptSession: (sid) => sid === 'current', onSession: () => {}, onStart: () => {}, onChunk: () => {}, onDone: () => {}, onError: () => {},
    onBackgroundTask: (task) => events.push(task), onBackgroundTasks: (tasks) => events.push(tasks),
  };
  dispatchWSMessage(JSON.stringify({type: 'background_tasks', sessionId: 'current', tasks: []}), handlers);
  dispatchWSMessage(JSON.stringify({type: 'background_task', sessionId: 'other', taskId: 'old', status: 'running'}), handlers);
  dispatchWSMessage(JSON.stringify({type: 'background_task', sessionId: 'current', taskId: 'new', processId: 'p', status: 'queued'}), handlers);
  assert.deepEqual(events, [[], {taskId: 'new', processId: 'p', status: 'queued'}]);
});

test('background command routes list and cancellation separately', async () => {
  const calls: string[] = [];
  const handlers = { background: (input: string) => calls.push(input) } as unknown as CliCommandHandlers;
  await runCliCommand('/background', handlers);
  await runCliCommand('/background stop', handlers);
  assert.deepEqual(calls, ['', 'stop']);
});
