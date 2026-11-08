import {test} from 'node:test';
import assert from 'node:assert/strict';
import {designTextError,designAnswersError,designLimits,characterCount} from '../v2/js/design/limits.js';
import {comparable} from '../v2/js/design/shared.js';

test('input limits preserve normal punctuation, code and emoji; reject invisible controls and malformed Unicode',()=>{
  for(const value of ['中文，标点！\n第二行\t第三行',`' <script>DROP TABLE students;</script>`, '👨‍👩‍👧‍👦🧑🏽‍🚀', '字'.repeat(4000)])assert.equal(designTextError(value,'内容',designLimits.answer),'');
  for(const value of [' \n\t','\u200d\ufe0f','\u0301','a\u0000','a\u202e','a\u2066','a\u200b','\ud800','\ufffd','字'.repeat(4001)])assert.notEqual(designTextError(value,'内容',designLimits.answer),'');
  assert.equal(characterCount('🧑🏽‍🚀'),4);
  assert.equal(designTextError('','要求',designLimits.message,false),'');
  assert.equal(comparable('方案\u200d\ufe0f'),comparable('方案'));
  assert.match(designAnswersError(['a'.repeat(4000),'b'.repeat(4000),'c'.repeat(4000),'d']),/12000/);
  assert.equal(designAnswersError(['a'.repeat(4000),'b'.repeat(4000),'c'.repeat(4000)]),'');
});
