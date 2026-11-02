// Real browser -> Go HTTP handlers -> isolated SQLite acceptance.
// Run from repository root: node --test frontend/tests/design-real.test.mjs
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createServer } from 'node:http';
import { readFile, writeFile } from 'node:fs/promises';
import { resolve, extname } from 'node:path';
import { chromium } from 'playwright';
import XLSX from 'xlsx';

let child, backend, browser, web, origin;
const results = [];
const report = (name, status, detail = '') => results.push({ name, status, detail });
const call = async (path, body, method = body ? 'POST' : 'GET') => {
  const response = await fetch(backend + '/api/v2' + path, { method, ...(body ? { headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body) } : {}) });
  return { status: response.status, ...await response.json() };
};
before(async () => {
  child = spawn('go', ['test', './internal/v2', '-run', '^TestDesignBrowserServer$', '-v', '-timeout', '5m'], { cwd: resolve('backend-go'), env: { ...process.env, DESIGN_BROWSER_TEST: '1', GOCACHE: resolve('backend-go/.cache/go-build') } });
  backend = await new Promise((done, fail) => {
    let output = '';
    const timer = setTimeout(() => fail(new Error('Fixture startup timeout: ' + output)), 90000);
    child.stdout.on('data', (chunk) => { output += chunk; const match = output.match(/DESIGN_BROWSER_URL=(http:\/\/[^\s]+)/); if (match) { clearTimeout(timer); done(match[1]); } });
    child.stderr.on('data', (chunk) => { output += chunk; });
    child.once('exit', (code) => { clearTimeout(timer); if (!backend) fail(new Error(`Fixture exited ${code}: ${output}`)); });
  });
  const root = resolve('frontend');
  web = createServer(async (req, res) => {
    try {
      const path = resolve(root, '.' + new URL(req.url, 'http://local').pathname);
      if (!path.startsWith(root + '/')) { res.writeHead(403).end(); return; }
      res.setHeader('Content-Type', {'.html':'text/html','.js':'text/javascript','.css':'text/css'}[extname(path)] || 'text/plain'); res.end(await readFile(path));
    } catch { res.writeHead(404).end(); }
  });
  await new Promise((done) => web.listen(0, '127.0.0.1', done)); origin = `http://127.0.0.1:${web.address().port}`;
  browser = await chromium.launch({ headless: true });
});
after(async () => {
  await writeFile('/private/tmp/design-acceptance-results.json', JSON.stringify(results, null, 2));
  await browser?.close();
  if (backend) await fetch(backend + '/__test/stop', { method: 'POST' }).catch(() => {});
  child?.kill();
  await new Promise((done) => web ? web.close(done) : done());
});

async function page(mockClipboard = true) {
  const p = await browser.newPage({ viewport: { width: 1280, height: 900 }, permissions: ['clipboard-read', 'clipboard-write'] });
  p.setDefaultTimeout(7000);
  await p.route('**/api/v2/**', async (route) => {
    const url = new URL(route.request().url());
    const response = await route.fetch({ url: backend + url.pathname + url.search });
    await route.fulfill({ response });
  });
  if (mockClipboard) await p.addInitScript(() => Object.defineProperty(navigator, 'clipboard', { value: { writeText: async (text) => { window.testCopy = text; } } }));
  return p;
}
async function teacher(p) {
  await p.goto(origin + '/teacher.html'); await p.locator('#passwordInput').fill('0506'); await p.locator('#passwordSubmitBtn').click();
  await p.locator('#mainContainer').waitFor({ state:'visible' }); await p.locator('[data-tab=design]').click(); await p.locator('#dpSave').waitFor();
}
async function login(p, name = '学生甲', classId = '1') {
  await p.goto(origin + '/design.html'); await p.locator('#designClass').selectOption(classId);
  await p.waitForFunction(() => !document.querySelector('#designEnter').disabled && document.querySelector('#designNames').options.length > 0);
  await p.locator('#designName').fill(name); await p.locator('#designEnter').click(); await p.locator('#designWorkspace').waitFor({ state:'visible' });
}
async function save(p) {
  await p.locator('#dpSave').click(); await p.waitForFunction(() => document.querySelector('#dpMessage')?.textContent === '方案保存成功。');
}

test('real full acceptance and defect probes', async (t) => {
  const tp = await page(), sp = await page();
  const errors = []; tp.on('pageerror', (e) => errors.push(e.message)); sp.on('pageerror', (e) => errors.push(e.message));
  let planID, extraID;
  try {
    await t.test('teacher native login, question add/delete/reorder, required fields, create and bind', async () => {
      await teacher(tp);
      assert.equal(await tp.locator('[data-dp-up="0"]').isDisabled(), true);
      assert.equal(await tp.locator('[data-dp-down="0"]').isDisabled(), true);
      report('截图中单题上移/下移禁用', 'PASS', '唯一一道题没有可移动位置，按钮禁用属于正常逻辑。');
      await tp.locator('#dpTitle').fill(' '); await tp.locator('[data-dp-question="0"]').fill(' '); await tp.locator('#dpSave').click();
      assert.match(await tp.locator('#dpMessage').innerText(), /填写标题/); assert.equal((await call('/design-plans')).data.plans.length, 0);
      await tp.locator('#dpTitle').fill('真实验收校园方案'); await tp.locator('[data-dp-question="0"]').fill('校园目标是什么？');
      await tp.locator('#dpAdd').click(); await tp.locator('[data-dp-question="1"]').fill('实施步骤是什么？');
      await tp.locator('#dpAdd').click(); await tp.locator('[data-dp-question="2"]').fill('临时问题');
      await tp.locator('[data-dp-remove="2"]').click(); assert.equal(await tp.locator('[data-dp-question]').count(), 2);
      await tp.locator('[data-dp-down="0"]').click(); assert.equal(await tp.locator('[data-dp-question="0"]').inputValue(), '实施步骤是什么？');
      await tp.locator('[data-dp-up="1"]').click(); await tp.locator('[data-dp-class="1"]').check(); await save(tp);
      const plans = (await call('/design-plans')).data.plans; planID = plans[0].id;
      assert.deepEqual(plans[0].questions, ['校园目标是什么？','实施步骤是什么？']); assert.deepEqual(plans[0].classIds, [1]);
      assert.equal(await tp.locator('.design-plan-card').count(), 1);
      assert.match(await tp.locator('.design-plan-card').innerText(), /真实验收校园方案/);
      await tp.locator('#dpNew').click();
      assert.match(await tp.locator('#dpSaveStatus').innerText(), /尚未保存/);
      assert.equal(await tp.locator('.design-plan-card').count(), 1);
      await tp.locator(`[data-dp-open="${planID}"]:not([data-dp-records])`).click();
      assert.equal(await tp.locator('#dpTitle').inputValue(), '真实验收校园方案');
      await tp.screenshot({path:'/private/tmp/design-teacher-library.png',fullPage:true});
      report('教师真实登录、新建、问题增删排序、必填、班级绑定', 'PASS');
    });
    if (!planID) return;
    await t.test('multiple plans, binding isolation, student identity and first version', async () => {
      extraID = (await call('/design-plans', { title:'第二个绑定方案', questions:['独立问题'], classIds:[1] })).data.plan.id;
      const other = await call('/design-plans', {title:'乙班独立方案',questions:['乙班问题'],classIds:[2]}); assert.equal(other.success, true);
      assert.equal((await call('/classes/2/design-plans?studentId=3')).data.plans.length, 1);
      assert.equal((await call(`/design-plans/${planID}/student?classId=2&studentId=3`)).status, 400);
      await sp.goto(origin + '/design.html'); await sp.locator('#designName').fill('不在名单'); await sp.locator('#designEnter').click();
      assert.match(await sp.locator('#designLoginMessage').innerText(), /完整姓名/);
      await login(sp); assert.equal(await sp.locator('#designPlanSelect option').count(), 3); assert.equal(await sp.locator('[data-answer]').count(), 0);
      await sp.locator('#designPlanSelect').selectOption(String(planID)); await sp.locator('[data-answer]').nth(1).waitFor();
      await sp.locator('[data-answer]').nth(0).fill('  '); await sp.locator('[data-answer]').nth(1).fill('分组实施'); await sp.locator('#designSubmit').click();
      assert.match(await sp.locator('#designSubmitMessage').innerText(), /不能为空/);
      await sp.locator('[data-answer]').nth(0).fill('绿色校园 <script>不执行</script>'); await sp.locator('#designSubmit').click(); await sp.locator('#copyFirst').waitFor();
      assert.equal(await sp.locator('[data-answer]').count(), 0); assert.equal(await sp.locator('#designContent script').count(), 0);
      await sp.locator('#copyFirst').click(); assert.match(await sp.evaluate(() => window.testCopy), /校园目标是什么？\n绿色校园/);
      const data = (await call(`/design-plans/${planID}/student?classId=1&studentId=1`)).data;
      assert.equal(data.versions.length, 1); assert.equal(data.versions[0].answers[0], '绿色校园 <script>不执行</script>');
      report('多方案选择、跨班隔离、名单校验、第一版落库及复制', 'PASS');
    });
    await t.test('persistent 120-second wait, changed second version, final read-only and relogin', async () => {
      await login(sp); await sp.locator('#designPlanSelect').selectOption(String(planID)); await sp.locator('#copyFirst').waitFor();
      assert.equal(await sp.locator('[data-answer]').count(), 0);
      const first = (await call(`/design-plans/${planID}/student?classId=1&studentId=1`)).data.versions[0];
      await sp.clock.install({time:new Date(first.submittedAt)}); await sp.clock.fastForward(121000);
      await sp.locator('[data-answer]').nth(1).waitFor(); assert.equal(await sp.locator('[data-answer]').nth(1).inputValue(), '分组实施');
      await sp.locator('#designSubmit').click(); assert.match(await sp.locator('#designSubmitMessage').innerText(), /至少优化修改/);
      await sp.locator('[data-answer]').nth(1).fill('分 组\n实施'); await sp.locator('#designSubmit').click(); assert.match(await sp.locator('#designSubmitMessage').innerText(), /至少优化修改/);
      await sp.locator('[data-answer]').nth(1).fill('分组实施，并每周检查效果'); await sp.locator('#designSubmit').click(); await sp.locator('#copyFinal').waitFor();
      assert.equal(await sp.locator('#designVersion').inputValue(), '2'); assert.equal(await sp.locator('[data-answer]').count(), 0);
      await sp.locator('#copyFinal').click(); assert.match(await sp.evaluate(() => window.testCopy), /每周检查/);
      await sp.locator('#designVersion').selectOption('1'); await sp.locator('#copyFinal').click(); assert.doesNotMatch(await sp.evaluate(() => window.testCopy), /每周检查/);
      await login(sp); await sp.locator('#designPlanSelect').selectOption(String(planID)); await sp.locator('#copyFinal').waitFor();
      assert.equal(await sp.locator('#designSubmit').count(), 0);
      const v = (await call(`/design-plans/${planID}/student?classId=1&studentId=1`)).data.versions;
      assert.equal(v.length,2);assert.equal(v[0].answers[1],'分组实施');assert.equal(v[1].answers[1],'分组实施，并每周检查效果');
      report('刷新/重登计时、第二版预填与变化校验、只读与版本复制', 'PASS', '浏览器虚拟时间跨越120秒，保存与状态读取均使用真实后端。');
    });
    await t.test('teacher lock, status filters, viewing and real Excel cells', async () => {
      await tp.locator('[data-tab=course]').click(); await tp.locator('[data-tab=design]').click(); await tp.locator('#dpSelect').selectOption(String(planID));
      await tp.waitForFunction(() => document.querySelectorAll('.design-record').length === 2);
      assert.equal(await tp.locator('#dpAdd').count(),0);assert.equal(await tp.locator('[data-dp-question="0"]').getAttribute('readonly'),'');
      await tp.locator('#dpStatusFilter').selectOption('2');assert.equal(await tp.locator('.design-record').count(),1);
      await tp.locator('.design-record summary').click();assert.match(await tp.locator('.design-record').innerText(),/每周检查/);
      await tp.locator('#dpStatusFilter').selectOption('0');assert.equal(await tp.locator('.design-record').count(),1);
      const file = tp.waitForEvent('download'); await tp.locator('#dpExport').click(); const workbook = XLSX.read(await readFile(await (await file).path()));
      const summary = XLSX.utils.sheet_to_json(workbook.Sheets['完成状态']); const details = XLSX.utils.sheet_to_json(workbook.Sheets['两版答案对照']);
      assert.equal(summary.length,2);assert.equal(details.length,4);assert.ok(details.some((r)=>r['第二版答案']==='分组实施，并每周检查效果'));
      assert.ok(summary.some((r)=>r['状态']==='未提交'));
      const original = (await call('/design-plans')).data.plans.find((p)=>p.id===planID);
      assert.equal((await call(`/design-plans/${planID}`,{...original,questions:['非法换题']},'PUT')).status,400);
      report('教师题目锁定、提交状态筛选、两版查看、真实Excel内容', 'PASS');
    });
    await t.test('duplicate submission and backend nonempty checks', async () => {
      const payload={classId:1,studentId:2,version:1,answers:['一次保存','步骤']};
      const attempts=await Promise.all([call(`/design-plans/${planID}/versions`,payload),call(`/design-plans/${planID}/versions`,payload)]);
      assert.deepEqual(attempts.map((r)=>r.status).sort(),[200,400]);
      assert.equal((await call(`/design-plans/${extraID}/versions`,{classId:1,studentId:2,version:1,answers:[' ']})).status,400);
      const rows=(await call(`/design-plans/${planID}/records`)).data.students;assert.equal(rows.find((s)=>s.studentId===2).versions.length,1);
      report('并发重复提交、后端非空与版本顺序保护', 'PASS');
    });
    await t.test('teacher unsaved edits and stale record probes', async () => {
      await tp.locator('#dpSelect').selectOption(String(extraID));await tp.locator('#dpTitle').fill('尚未保存的标题');await tp.locator('[data-dp-question="0"]').fill('尚未保存的问题');
      await tp.locator('[data-tab=course]').click();await tp.locator('[data-tab=design]').click();
      const retained=await tp.locator('#dpTitle').inputValue()==='尚未保存的标题';
      assert.equal(await tp.locator('[data-dp-question="0"]').inputValue(),'尚未保存的问题');
      report('教师切换页签保留未保存配置',retained?'PASS':'FAIL',retained?'页签切换后标题、问题仍保留。':'切换后未保存配置丢失。');
      const cancelSwitch=async(dialog)=>dialog.dismiss();tp.once('dialog',cancelSwitch);
      await tp.locator('#dpSelect').selectOption(String(planID));assert.equal(await tp.locator('#dpSelect').inputValue(),String(extraID));
      tp.once('dialog',(dialog)=>dialog.accept());
      await tp.locator('#dpSelect').selectOption(String(planID)); await tp.waitForFunction(()=>document.querySelector('#dpRecordMessage')?.textContent==='共 2 名学生');
      await tp.route(`**/api/v2/design-plans/${extraID}/records`,async (route)=>{await new Promise((r)=>setTimeout(r,800));await route.fallback();});
      await tp.locator('#dpSelect').selectOption(String(extraID));await tp.locator('#dpStatusFilter').selectOption('1');
      const stale=await tp.locator('.design-record').count()>0;
      report('方案切换加载期间记录隔离',stale?'FAIL':'PASS',stale?'加载期间错误显示上一方案的记录。':'加载期间切换筛选不再出现上个方案的记录。');
      await tp.waitForTimeout(900);
    });
    await t.test('student submitting input consistency probe', async () => {
      await login(sp,'学生乙');await sp.locator('#designPlanSelect').selectOption(String(extraID));await sp.locator('[data-answer]').waitFor();
      await sp.locator('[data-answer]').fill('点击提交前的答案');
      await sp.route(`**/api/v2/design-plans/${extraID}/versions`,async(route)=>{await new Promise((r)=>setTimeout(r,700));await route.fallback();});
      await sp.locator('#designSubmit').click();
      const input=sp.locator('[data-answer]');const editable=await input.isEditable();if(editable)await input.fill('提交过程中继续编辑的新答案');
      await sp.locator('#copyFirst').waitFor();
      const saved=(await call(`/design-plans/${extraID}/student?classId=1&studentId=2`)).data.versions[0].answers[0];
      report('学生提交期间输入保护',editable&&saved!=='提交过程中继续编辑的新答案'?'FAIL':'PASS',editable?'提交过程中仍能输入，后续输入丢失。':'提交期间答案输入框已禁用，避免产生未保存的新输入。');
    });
    await t.test('mobile layout and runtime errors',async()=>{
      for(const p of [tp,sp]){await p.setViewportSize({width:390,height:844});assert.ok(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1));}
      assert.deepEqual(errors,[]);report('移动端横向布局、页面运行异常','PASS');
      await tp.screenshot({path:'/private/tmp/design-acceptance-teacher.png',fullPage:true});await sp.screenshot({path:'/private/tmp/design-acceptance-student.png',fullPage:true});
    });
    await t.test('real browser clipboard API and HTTP-compatible copy fallback',async()=>{
      const cp=await page(false);
      let previous;
      try {
        await login(cp);await cp.locator('#designPlanSelect').selectOption(String(planID));await cp.locator('#copyFinal').waitFor();
        previous=await cp.evaluate(()=>navigator.clipboard.readText());
        await cp.locator('#copyFinal').click();
        const copied=await cp.evaluate(()=>navigator.clipboard.readText());assert.match(copied,/第二版方案/);assert.match(copied,/校园目标是什么？/);assert.match(copied,/每周检查/);
        await cp.evaluate(()=>{window.nativeClipboard=navigator.clipboard;Object.defineProperty(navigator,'clipboard',{value:undefined,configurable:true});});
        await cp.locator('#designVersion').selectOption('1');await cp.locator('#copyFinal').click();
        const fallback=await cp.evaluate(()=>window.nativeClipboard.readText());assert.match(fallback,/第一版方案/);assert.doesNotMatch(fallback,/每周检查/);
        report('实际剪贴板读写与无 Clipboard API 时的复制兼容','PASS');
      } finally {if(previous!==undefined)await cp.evaluate((text)=>(window.nativeClipboard||navigator.clipboard).writeText(text),previous).catch(()=>{});await cp.close();}
    });
    await t.test('class filter, binding change and historical records',async()=>{
      await tp.locator('#dpSelect').selectOption(String(planID));
      await tp.locator('#dpClassFilter').selectOption('2');
      await tp.waitForFunction(()=>document.querySelector('#dpRecordMessage')?.textContent==='共 0 名学生');
      const original=(await call('/design-plans')).data.plans.find((p)=>p.id===planID);
      const changed=await call(`/design-plans/${planID}`,{...original,classIds:[2]},'PUT');assert.equal(changed.success,true);
      const history=(await call(`/design-plans/${planID}/records?classId=1`)).data.students;
      assert.equal(history.length,2);assert.equal(history[0].versions.length+history[1].versions.length,3);
      assert.equal((await call(`/design-plans/${planID}/student?classId=1&studentId=1`)).status,400);
      const wb=XLSX.read(Buffer.from(await (await fetch(backend+`/api/v2/design-plans/${planID}/export?classId=1`)).arrayBuffer()));
      assert.equal(XLSX.utils.sheet_to_json(wb.Sheets['完成状态']).length,2);
      report('班级筛选、修改绑定、解绑保留历史记录和导出','PASS');
    });
    await t.test('acceptance gate: no reproduced defects',()=>{
      assert.deepEqual(results.filter((r)=>r.status==='FAIL'),[], 'Acceptance is blocked by the reproduced defects listed in the report.');
    });
  } finally { await tp.close();await sp.close(); console.log(JSON.stringify(results,null,2)); }
});
