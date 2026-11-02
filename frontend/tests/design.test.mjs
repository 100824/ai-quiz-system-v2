// Run with: node --test frontend/tests/design.test.mjs
// Browser tests use mock APIs; no classroom database or external AI is accessed.
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { resolve, extname } from 'node:path';
import { chromium } from 'playwright';

let browser, server, origin;
before(async () => {
  const root = resolve('frontend');
  server = createServer(async (req, res) => {
    try {
      const path = resolve(root, `.${new URL(req.url, 'http://localhost').pathname}`);
      if (!path.startsWith(root + '/')) { res.writeHead(403).end(); return; }
      res.setHeader('Content-Type', ({ '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css' })[extname(path)] || 'text/plain');
      res.end(await readFile(path));
    } catch { res.writeHead(404).end(); }
  });
  await new Promise((done) => server.listen(0, '127.0.0.1', done));
  origin = `http://127.0.0.1:${server.address().port}`;
  browser = await chromium.launch({ headless: true });
});
after(async () => { await browser?.close(); await new Promise((done) => server ? server.close(done) : done()); });

async function fixture(page) {
  const plan = { id: 1, title: '校园设计', questions: ['目标是什么？', '如何实现？'], classIds: [1], locked: false };
  const versions = [];
  let failedSubmit = false;
  await page.addInitScript(() => {
    Object.defineProperty(navigator, 'clipboard', { value: { writeText: async (text) => { window.copiedPlan = text; } } });
  });
  await page.route('**/api/v2/**', async (route) => {
    const request = route.request(), path = new URL(request.url()).pathname;
    let data;
    if (path === '/api/v2/classes') data = { classes: [{ id: 1, name: '五年（1）班' }] };
    else if (path === '/api/v2/default-class') data = { defaultClassId: 1 };
    else if (path.endsWith('/students')) data = { students: [{ id: 7, name: '测试学生', classId: 1 }] };
    else if (path === '/api/v2/classes/1/design-plans') data = { plans: [{ id: 1, title: plan.title, versionCount: versions.length }] };
    else if (path.endsWith('/student')) data = { plan, versions };
    else if (path.endsWith('/versions')) {
      if (failedSubmit) { failedSubmit = false; await route.fulfill({ status: 500, json: { success: false, error: '模拟网络失败' } }); return; }
      const body = request.postDataJSON();
      assert.equal(body.version, versions.length + 1);
      versions.push({ version: body.version, title: plan.title, questions: [...plan.questions], answers: body.answers, submittedAt: new Date().toISOString() });
      data = null;
    } else if (path === '/api/v2/design-plans') data = { plans: [plan] };
    else if (path.endsWith('/records')) data = { students: [{ studentId: 7, classId: 1, name: '测试学生', className: '五年（1）班', versions }] };
    else if (path.endsWith('/export')) { await route.fulfill({ contentType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', body: 'mock export' }); return; }
    else if (request.method() === 'PUT') { Object.assign(plan, request.postDataJSON()); data = { plan }; }
    else throw new Error(`Unexpected API: ${request.method()} ${path}`);
    await route.fulfill({ json: { success: true, data } });
  });
  return { plan, versions, failNext: () => { failedSubmit = true; } };
}

async function login(page) {
  await page.locator('#designName').fill('测试学生');
  await page.locator('#designEnter').click();
  await page.locator('#designWorkspace').waitFor({ state: 'visible' });
}

test('student: validation, failure retention, persistent timer, immutable versions and copy', async () => {
  const page = await browser.newPage();
  const runtimeErrors = [];
  page.on('pageerror', (error) => runtimeErrors.push(error.message));
  try {
    const f = await fixture(page);
    await page.goto(`${origin}/design.html`);
    await page.locator('#designEnter').waitFor({ state: 'visible' });
    await page.locator('#designName').fill('错误姓名'); await page.locator('#designEnter').click();
    assert.match(await page.locator('#designLoginMessage').innerText(), /完整姓名/);
    await login(page);
    const answers = page.locator('[data-answer]');
    await answers.nth(0).fill('  '); await answers.nth(1).fill('实施步骤');
    await page.locator('#designSubmit').click();
    assert.match(await page.locator('#designSubmitMessage').innerText(), /不能为空/);
    await answers.nth(0).fill('校园环保');
    f.failNext(); await page.locator('#designSubmit').click();
    await page.waitForFunction(() => document.querySelector('#designSubmitMessage')?.textContent.includes('模拟网络失败'));
    assert.equal(await answers.nth(0).inputValue(), '校园环保'); assert.equal(f.versions.length, 0);
    await page.locator('#designSubmit').click();
    await page.locator('#copyFirst').waitFor();
    assert.equal(await page.locator('[data-answer]').count(), 0);
    await page.locator('#copyFirst').click();
    assert.match(await page.evaluate(() => window.copiedPlan), /目标是什么？\n校园环保/);
    await page.evaluate(() => {
      navigator.clipboard.writeText = async () => { throw new Error('clipboard unavailable'); };
      document.execCommand = () => { window.copiedPlan = document.activeElement.value; return true; };
    });
    await page.locator('#copyFirst').click();
    assert.match(await page.evaluate(() => window.copiedPlan), /第一版方案/);
    await page.evaluate(() => { document.execCommand = () => false; });
    await page.locator('#copyFirst').click();
    assert.match(await page.locator('#designNotice').innerText(), /复制失败/);
    await page.evaluate(() => { navigator.clipboard.writeText = async (text) => { window.copiedPlan = text; }; });
    const timestamp = f.versions[0].submittedAt;
    await page.reload(); await login(page);
    assert.equal(f.versions[0].submittedAt, timestamp);
    assert.equal(await page.locator('[data-answer]').count(), 0);
    await page.clock.install(); await page.clock.fastForward(121000);
    await page.locator('[data-answer]').first().waitFor();
    assert.equal(await page.locator('[data-answer]').first().inputValue(), '校园环保');
    await page.locator('[data-answer]').first().fill('校园 环保');
    await page.locator('#designSubmit').click(); assert.match(await page.locator('#designSubmitMessage').innerText(), /至少优化修改/);
    await page.locator('[data-answer]').first().fill('校园环保，增加垃圾分类'); await page.locator('#designSubmit').click();
    try { await page.locator('#designVersion').waitFor({ timeout: 5000 }); }
    catch (error) {
      console.log('Submission diagnostic:', JSON.stringify({ runtimeErrors, versions: f.versions, ui: await page.locator('#designContent').innerText(), inputs: await page.locator('[data-answer]').evaluateAll((nodes) => nodes.map((n) => ({ value: n.value, disabled: n.disabled }))) }));
      throw error;
    }
    assert.equal(await page.locator('#designVersion').inputValue(), '2');
    assert.equal(await page.locator('[data-answer]').count(), 0); assert.equal(f.versions.length, 2); assert.equal(f.versions[0].answers[0], '校园环保');
    await page.locator('#copyFinal').click(); assert.match(await page.evaluate(() => window.copiedPlan), /增加垃圾分类/);
    await page.locator('#designVersion').selectOption('1'); await page.locator('#copyFinal').click();
    assert.doesNotMatch(await page.evaluate(() => window.copiedPlan), /增加垃圾分类/);
    await page.reload(); await login(page); await page.locator('#copyFinal').waitFor();
    assert.equal(await page.locator('#designSubmit').count(), 0);
    await page.screenshot({ path: '/private/tmp/design-student-complete.png', fullPage: true });
  } finally { await page.close(); }
});

test('teacher: question order, bindings, lock, status filtering and export action', async () => {
  const page = await browser.newPage();
  try {
    const f = await fixture(page);
    await page.goto(`${origin}/teacher.html`);
    // Exercise the independent teacher module without invoking unrelated classroom APIs.
    await page.evaluate(async () => {
      document.querySelector('#mainContainer').classList.remove('hidden'); document.querySelector('#passwordOverlay').remove();
      document.querySelectorAll('.tab-content').forEach((n) => n.classList.remove('active')); document.querySelector('#design').classList.add('active');
      const module = await import('./v2/js/design/teacher.js?v=2026110203'); await module.loadDesignTeacher();
    });
    await page.locator('[data-dp-down="0"]').click();
    assert.equal(await page.locator('[data-dp-question="0"]').inputValue(), '如何实现？');
    await page.locator('#dpSaveTop').click(); await page.waitForFunction(() => document.querySelector('#dpMessage')?.textContent.includes('保存成功'));
    assert.equal(await page.locator('.design-plan-card').count(), 1);
    assert.match(await page.locator('.design-plan-card').innerText(), /校园设计/);
    await page.locator('[data-dp-open="1"][data-dp-records]').click();
    await page.waitForFunction(() => document.querySelector('#dpRecordMessage')?.textContent === '共 1 名学生');
    assert.deepEqual(f.plan.questions, ['如何实现？', '目标是什么？']); assert.deepEqual(f.plan.classIds, [1]);
    f.plan.locked = true;
    f.versions.push({ version: 1, title: f.plan.title, questions: f.plan.questions, answers: ['实现办法', '计划目标'], submittedAt: new Date().toISOString() });
    await page.evaluate(async () => { await (await import('./v2/js/design/teacher.js?v=2026110203')).loadDesignTeacher(); });
    assert.equal(await page.locator('#dpAdd').count(), 0);
    assert.equal(await page.locator('[data-dp-question="0"]').getAttribute('readonly'), '');
    await page.locator('#dpStatusFilter').selectOption('2'); assert.equal(await page.locator('.design-record').count(), 0);
    await page.locator('#dpStatusFilter').selectOption('1'); await page.locator('.design-record summary').click();
    assert.match(await page.locator('.design-record').innerText(), /实现办法/);
    const download = page.waitForEvent('download'); await page.locator('#dpExport').click(); assert.match((await download).suggestedFilename(), /\.xlsx$/);
    await page.screenshot({ path: '/private/tmp/design-teacher-records.png', fullPage: true });
  } finally { await page.close(); }
});

test('student: multiple bound plans require selection and remain separate; empty class is explained', async () => {
  const page = await browser.newPage();
  try {
    await fixture(page);
    await page.route('**/api/v2/classes/1/design-plans?*', (route) => route.fulfill({ json: { success: true, data: { plans: [{id:1,title:'校园设计',versionCount:0},{id:2,title:'另一个方案',versionCount:0}] } } }));
    await page.route('**/api/v2/design-plans/2/student?*', (route) => route.fulfill({ json: { success: true, data: { plan: {id:2,title:'另一个方案',questions:['独立的问题']}, versions: [] } } }));
    await page.goto(`${origin}/design.html`); await login(page);
    assert.equal(await page.locator('[data-answer]').count(), 0);
    await page.locator('#designPlanSelect').selectOption('2'); await page.locator('[data-answer]').waitFor();
    assert.match(await page.locator('#designContent').innerText(), /独立的问题/);
    await page.locator('#designPlanSelect').selectOption('1'); await page.locator('[data-answer]').nth(1).waitFor();
    assert.doesNotMatch(await page.locator('#designContent').innerText(), /独立的问题/);
    await page.setViewportSize({ width: 390, height: 844 });
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), 'student page overflows mobile viewport');
    await page.route('**/api/v2/classes/1/design-plans?*', (route) => route.fulfill({ json: { success: true, data: { plans: [] } } }));
    await page.reload(); await login(page);
    assert.match(await page.locator('#designContent').innerText(), /尚未绑定方案/);
  } finally { await page.close(); }
});

test('student: successful save followed by failed reload offers read retry, not duplicate submit', async () => {
  const page = await browser.newPage();
  try {
    const f = await fixture(page);
    let failRead = false;
    await page.route('**/api/v2/design-plans/1/student?*', (route) => failRead
      ? route.fulfill({ status: 503, json: { success: false, error: '暂时无法读取' } })
      : route.fallback());
    await page.goto(`${origin}/design.html`); await login(page);
    await page.locator('[data-answer]').nth(0).fill('初版目标'); await page.locator('[data-answer]').nth(1).fill('初版做法');
    failRead = true; await page.locator('#designSubmit').click();
    await page.getByRole('button', { name: '重新加载方案' }).waitFor();
    assert.equal(f.versions.length, 1); assert.equal(await page.locator('#designSubmit').count(), 0);
    assert.match(await page.locator('#designNotice').innerText(), /提交成功/);
    failRead = false; await page.getByRole('button', { name: '重新加载方案' }).click();
    await page.locator('#copyFirst').waitFor(); assert.equal(f.versions.length, 1);
  } finally { await page.close(); }
});

test('visual states: desktop login, writing, AI wait, second draft and mobile layout', async () => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  try {
    const f = await fixture(page);
    await page.goto(`${origin}/design.html`);
    await page.waitForFunction(() => document.querySelector('#designNames').options.length > 0);
    await page.screenshot({ path: '/private/tmp/design-login-desktop.png', fullPage: true });
    await login(page);
    assert.equal(await page.locator('.design-stepper [aria-current="step"]').innerText(), '1\n填写第一版');
    await page.locator('[data-answer]').nth(0).fill('让校园更环保，让同学们养成垃圾分类的好习惯。');
    await page.locator('[data-answer]').nth(1).fill('分组设计分类标识，安排同学每周检查实施效果。');
    await page.screenshot({ path: '/private/tmp/design-writing-desktop.png', fullPage: true });
    await page.locator('#designSubmit').click(); await page.locator('#copyFirst').waitFor();
    assert.doesNotMatch(await page.locator('#designContent').innerText(), /AI 优化中|第一版已保存|第一版会完整保留/);
    assert.equal(await page.locator('.design-state-badge').count(), 0);
    await page.screenshot({ path: '/private/tmp/design-wait-desktop.png', fullPage: true });
    assert.equal(await page.locator('.design-stepper [aria-current="step"]').innerText(), '2\n与 AI 一起优化');
    f.versions[0].submittedAt = new Date(Date.now() - 121000).toISOString();
    await page.reload(); await login(page); await page.locator('[data-answer]').nth(1).waitFor();
    assert.equal(await page.locator('.design-stepper [aria-current="step"]').innerText(), '3\n提交第二版');
    await page.screenshot({ path: '/private/tmp/design-second-desktop.png', fullPage: true });
    await page.setViewportSize({ width: 390, height: 844 });
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1));
    await page.screenshot({ path: '/private/tmp/design-writing-mobile.png', fullPage: true });
    await page.goto(`${origin}/design.html`);
    await page.locator('#designEnter').waitFor();
    const position = await page.locator('#designEnter').boundingBox(); assert.ok(position.y < 844, 'mobile login action should be visible without scrolling');
    await page.screenshot({ path: '/private/tmp/design-login-mobile.png', fullPage: true });
  } finally { await page.close(); }
});
