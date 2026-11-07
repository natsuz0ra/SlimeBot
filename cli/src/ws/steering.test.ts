import assert from 'node:assert/strict';
import test from 'node:test';
import { dispatchWSMessage, type WSHandlers } from './socket';

test('CLI routes steering acknowledgements independently of stream completion', () => {
  const changes: unknown[] = [];
  let turn = '';
  const handlers: WSHandlers = {
    onSession: () => {}, onStart: (_sid, id) => {turn = id || '';}, onChunk: () => {},
    onDone: () => {throw new Error('steering must not end the stream');}, onError: () => {throw new Error('steering must not fail the stream');},
    onSteering: (change) => changes.push(change),
  };
  dispatchWSMessage(JSON.stringify({type: 'start', turnId: 'turn-1'}), handlers);
  dispatchWSMessage(JSON.stringify({type: 'steer_accepted', change: {content: 'new guidance'}}), handlers);
  dispatchWSMessage(JSON.stringify({type: 'steer_rejected', error: 'stale'}), handlers);
  assert.equal(turn, 'turn-1');
  assert.deepEqual(changes, [{content: 'new guidance'}, {error: 'stale'}]);
});
