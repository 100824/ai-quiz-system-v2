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
  await writeFile('/private/tmp/design-html-acceptance-results.json', JSON.stringify(results, null, 2));
  await browser?.close();
  if (backend) await fetch(backend + '/__test/stop', { method: 'POST' }).catch(() => {});
  child?.kill();
  await new Promise((done) => web ? web.close(done) : done());
});

async function page(mockClipboard = true) {
  const p = await browser.newPage({ viewport: { width: 1280, height: 900 }, permissions: ['clipboard-read', 'clipboard-write'] });
  p.setDefaultTimeout(7000);
  await p.context().route('**/api/v2/**', async (route) => {
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
async function activate(p, id, name = '学生甲') { await call('/classes/1/design-active-plan',{planId:id},'PUT'); await login(p,name); }
async function save(p) {
  await p.locator('#dpSave').click(); await p.waitForFunction(() => document.querySelector('#dpMessage')?.textContent === '方案保存成功。');
}

test('HTML design creation, preview, modifications, final choice and export',async(t)=>{
 const tp=await page(),sp=await page();const errors=[];tp.on('pageerror',e=>errors.push(e.message));sp.on('pageerror',e=>errors.push(e.message));let id,firstId;
 try {
  await t.test('teacher HTML type and two-version workflow',async()=>{
   await teacher(tp);await tp.locator('#dpTitle').fill('离线校园互动网页');await tp.locator('#dpOutputType').selectOption('html');
   await tp.locator('[data-dp-question="0"]').fill('想做什么网页？');await tp.locator('#dpAdd').click();await tp.locator('[data-dp-question="1"]').fill('如何交互？');
   await tp.locator('[data-dp-class="1"]').check();await save(tp);id=(await call('/design-plans')).data.plans[0].id;
   assert.equal((await call('/design-plans')).data.plans[0].outputType,'html');
   await call('/classes/1/design-active-plan',{planId:id},'PUT');await login(sp);
   await sp.locator('[data-answer]').nth(0).fill('校园计数器');await sp.locator('[data-answer]').nth(1).fill('点击按钮加一');await sp.locator('#designSubmit').click();await sp.locator('#copyFirst').waitFor();
   await call(`/design-plans/${id}/classes/1/progress`,{stage:2},'PUT');await sp.locator('[data-answer]').nth(1).waitFor();
   await sp.locator('[data-answer]').nth(1).fill('点击加一按钮，显示总数');await sp.locator('#designSubmit').click();await sp.locator('#copyFinal').waitFor();
   assert.match(await sp.locator('#designContent').innerText(),/等待老师开启网页环节/);
   await tp.locator('#dpProgressRefresh').click();await tp.waitForFunction(()=>document.querySelector('#dpAdvance').textContent==='开启网页环节');await tp.locator('#dpAdvance').click();
   await sp.locator('#imageGenerate').waitFor();await sp.waitForFunction(()=>!document.querySelector('#imageGenerate').disabled);
   assert.match(await sp.locator('#imageGenerate').innerText(),/生成网页/);
   assert.equal((await(await fetch(backend+'/__test/image-calls')).json()).calls,0);
  });
  if(!id)return;
  await t.test('mode changes preserve unsent creation content and require confirmation',async()=>{
   await sp.locator('[data-image-answer]').nth(0).fill('未提交的生成内容');await sp.locator('#imageInstruction').fill('请突出按钮');
   const plan=(await call('/design-plans')).data.plans.find(p=>p.id===id);
   await call(`/design-plans/${id}`,{...plan,outputType:'image'},'PUT');
   await sp.waitForFunction(()=>document.querySelector('#imageGenerate')?.textContent==='生成图片');
   assert.equal(await sp.locator('[data-image-answer]').nth(0).inputValue(),'未提交的生成内容');assert.equal(await sp.locator('#imageInstruction').inputValue(),'请突出按钮');
   await call(`/design-plans/${id}`,{...plan,outputType:'html'},'PUT');
   await sp.waitForFunction(()=>document.querySelector('#imageGenerate')?.textContent==='生成网页');
   assert.equal(await sp.locator('[data-image-answer]').nth(0).inputValue(),'未提交的生成内容');assert.equal(await sp.locator('#imagePlanConfirmed').isChecked(),false);
   await sp.locator('#imageGenerate').click();assert.match(await sp.locator('#imageFeedback').innerText(),/确认/);
  });
  await t.test('explicit first generation and same-origin isolation in a new tab',async()=>{
   await sp.locator('#imagePlanConfirmed').check();await sp.locator('#imageGenerate').click();await sp.locator('#imageGenerating').waitFor({state:'visible'});
   await sp.waitForFunction(()=>document.querySelector('#htmlThinkingText')?.textContent.includes('先设计布局与按钮交互'));
   assert.equal(await sp.locator('#htmlThinking').evaluate(el=>el.open),true);
   assert.equal(await sp.locator('#htmlThinkingText img').count(),0);assert.equal(await sp.evaluate(()=>window.reasoningXSS),undefined);
   assert.match(await sp.locator('#imageGeneratingTitle').innerText(),/思考/);
   await login(sp);await sp.locator('#htmlThinking').waitFor({state:'visible'});
   assert.match(await sp.locator('#htmlThinkingText').innerText(),/先设计布局与按钮交互/);
   await sp.waitForFunction(()=>document.querySelectorAll('#imageGallery input').length===1,null,{timeout:12000});firstId=await sp.locator('#imageGallery input').first().inputValue();
   assert.equal(await sp.locator('#htmlThinking').evaluate(el=>el.open),false);
   await sp.locator('#htmlThinking summary').click();assert.equal(await sp.locator('#htmlThinkingText').isVisible(),true);
   assert.equal((await call('/design-plans')).data.plans.find(p=>p.id===id).outputTypeLocked,true);
   await sp.locator('#imageInstruction').fill('保留在原页面的文字');
   const popup=sp.waitForEvent('popup');await sp.locator('#imageLatestOpen').click();const view=await popup;await view.waitForLoadState();await view.locator('#previewFrame').waitFor();
   assert.equal(await view.evaluate(()=>window.opener),null);assert.equal(await view.locator('#previewFrame').getAttribute('sandbox'),'allow-scripts');
   const frame=view.frameLocator('#previewFrame');await frame.locator('#add').click();assert.equal(await frame.locator('#score').innerText(),'1');
   assert.equal(await frame.locator('body').evaluate(()=>{try{return !!parent.document.body}catch{return false}}),false);
   assert.equal(await frame.locator('body').evaluate(async()=>{try{await fetch('http://127.0.0.1:8080/api/v2/classes');return true}catch{return false}}),false);
   assert.equal(await sp.locator('#imageInstruction').inputValue(),'保留在原页面的文字');
   await view.reload();await view.frameLocator('#previewFrame').locator('#add').click();assert.equal(await view.frameLocator('#previewFrame').locator('#score').innerText(),'1');
   await view.screenshot({path:'/private/tmp/design-html-new-tab.png',fullPage:true});
   const fileEvent=view.waitForEvent('download');await view.locator('#previewDownload').click();const file=await fileEvent;
   assert.match(file.suggestedFilename(),/\.html$/);const source=await readFile(await file.path(),'utf8');assert.match(source,/connect-src/);
   const offlinePath='/private/tmp/design-html-offline.html';await file.saveAs(offlinePath);
   const offline=await browser.newPage();await offline.context().setOffline(true);await offline.goto('file://'+offlinePath);await offline.locator('#add').click();assert.equal(await offline.locator('#score').innerText(),'1');await offline.close();
   await view.close();
  });
  await t.test('successive edit, old version remains, failure retention and final lock',async()=>{
   await sp.locator('#imageInstruction').fill('标题改成蓝色校园');await sp.locator('#imageGenerate').click();
   await sp.waitForFunction(()=>document.querySelectorAll('#imageGallery input').length===2,null,{timeout:12000});
   const state=(await call(`/design-plans/${id}/html?classId=1&studentId=1`)).data;
   assert.equal(state.latestVersion.parentVersionId,firstId);assert.equal(state.messages.length,0);
   await sp.locator('#imageInstruction').fill('模拟失败');await sp.locator('#imageGenerate').click();await sp.waitForFunction(()=>document.querySelector('#imageFeedback')?.textContent.includes('网页生成失败'),null,{timeout:12000});
   assert.equal(await sp.locator('#imageInstruction').inputValue(),'模拟失败');assert.equal(await sp.locator('#imageGallery input').count(),2);
   await sp.locator('#imageGallery input').first().check();sp.once('dialog',d=>d.accept());await sp.locator('#imageFinalize').click();await sp.waitForFunction(()=>document.querySelector('#imageFinalStatus')?.textContent.includes('已提交最终作品'));
   assert.equal(await sp.locator('#imageCompose').isVisible(),false);
   await login(sp);await sp.locator('#imageLatestOpen').waitFor();assert.equal(await sp.locator('#imageCompose').isVisible(),false);
   const final=(await call(`/design-plans/${id}/html?classId=1&studentId=1`)).data;assert.equal(final.finalVersionId,firstId);
   assert.match(await sp.locator('#imageLatestOpen').getAttribute('href'),new RegExp(firstId));
   await sp.locator('#imageGallery .design-html-thumb').first().scrollIntoViewIfNeeded();await sp.locator('#imageGallery iframe').first().waitFor();
   await sp.evaluate(()=>window.scrollTo(0,0));await sp.waitForFunction(()=>document.querySelectorAll('#imageGallery iframe').length===0);
  });
  await t.test('teacher history, type lock, real ZIP files and mobile layout',async()=>{
   await tp.locator('[data-tab=course]').click();await tp.locator('[data-tab=design]').click();await tp.locator('#dpSelect').selectOption(String(id));await tp.waitForFunction(()=>document.querySelector('#dpOutputType')?.disabled===true);
   await tp.locator('#dpStatusFilter').selectOption('final');await tp.locator('.design-record summary').click();await tp.locator('[data-image-student="1"]').click();
   await tp.locator('[data-image-details="1"] .design-image-thumb').nth(1).waitFor();assert.match(await tp.locator('[data-image-details="1"]').innerText(),/最终作品/);assert.match(await tp.locator('[data-image-details="1"]').innerText(),/模拟失败/);
   const download=tp.waitForEvent('download');await tp.locator('#dpImageExport').click();const file=await download;const zip=XLSX.CFB.read(await readFile(await file.path()),{type:'buffer'});assert.equal(zip.FullPaths.filter(p=>p.endsWith('.html')).length,2);
   await sp.screenshot({path:'/private/tmp/design-html-versions.png',fullPage:true});await sp.setViewportSize({width:390,height:844});await sp.screenshot({path:'/private/tmp/design-html-mobile.png',fullPage:true});
   assert.equal(await sp.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);assert.deepEqual(errors,[]);
  });
 } finally {await sp.close();await tp.close();}
});
