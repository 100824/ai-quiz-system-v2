// Generated-page progress UI, including providers that omit reasoning entirely.
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
    const path = new URL(req.url, 'http://local').pathname;
    if (path === '/__thinking') {
      res.setHeader('Content-Type', 'text/html');
      res.end('<!doctype html><html lang="zh-CN"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/v2/css/tokens.css"><link rel="stylesheet" href="/v2/css/app.css"><link rel="stylesheet" href="/v2/css/design.css"><body class="student-page design-page"><main class="app-shell"><div id="fixture"></div></main></body></html>');
      return;
    }
    try {
      const file = resolve(root, '.' + path);
      if (!file.startsWith(root + '/')) { res.writeHead(403).end(); return; }
      res.setHeader('Content-Type', {'.js':'text/javascript','.css':'text/css'}[extname(file)] || 'text/plain');
      res.end(await readFile(file));
    } catch { res.writeHead(404).end(); }
  });
  await new Promise(done => server.listen(0, '127.0.0.1', done));
  origin = `http://127.0.0.1:${server.address().port}`;
  browser = await chromium.launch({headless:true});
});
after(async () => { await browser?.close(); await new Promise(done => server.close(done)); });

async function fixture(outputType = 'html', initialJob = null, questionCount = 1) {
  const page = await browser.newPage({viewport:{width:1100,height:850}});
  await page.clock.install();
  let job = initialJob, reads = 0, posts = [];
  const version = {id:'web1',version:1,model:'fixture',createdAt:new Date().toISOString()};
  await page.route('**/api/v2/**', async route => {
    const path = new URL(route.request().url()).pathname;
    let data;
    if (route.request().method() === 'POST') {posts.push(route.request().postDataJSON());data = {job};}
    else if (path.includes('/html-versions/')) data = {version,html:'<!doctype html><html><head><title>测试</title></head><body>测试网页</body></html>'};
    else {
      reads++;
      const complete = job?.status === 'succeeded';
      data = {versionCount:complete?1:0,versions:complete?[version]:[],messages:[],versionsHasMore:false,messagesHasMore:false,activeJob:['running','queued'].includes(job?.status)?job:null,lastJob:job,latestVersion:complete?version:null,finalVersionId:'',imageCount:0,images:[],imagesHasMore:false,latestImage:null};
    }
    await route.fulfill({json:{success:true,data}});
  });
  await page.goto(origin + '/__thinking');
  await page.evaluate(async ({type,questionCount}) => {
    const {mountDesignImages} = await import('/v2/js/design/images.js');
    window.stopCreationTest = mountDesignImages({node:document.querySelector('#fixture'),planId:1,classId:1,studentId:1,title:'我的互动网页',outputType:type,versions:[1,2].map(version=>({version,title:'我的互动网页',questions:Array.from({length:questionCount},()=> '设计什么？'),answers:Array.from({length:questionCount},()=> '带按钮的校园小测')}))});
  }, {type:outputType,questionCount});
  await page.waitForFunction(()=>!document.querySelector('#imageGenerate').disabled || !document.querySelector('#imageGenerating').classList.contains('hidden'));
  return {page, posts, setJob:next=>{job=next;}, tick:async()=>{const previous=reads;await page.clock.fastForward(3001);const deadline=Date.now()+5000;while(reads===previous) {if(Date.now()>deadline) throw new Error('Progress polling did not run');await new Promise(done=>setTimeout(done,10));}}};
}

for(const mode of ['image','html']) test(`${mode}: overlong source, aggregate answers and control characters never enqueue a generation`,async()=>{
  const f=await fixture(mode,null,4),p=f.page;
  try{
    const fields=p.locator('[data-image-answer]');
    await fields.nth(0).fill('字'.repeat(4001));await p.locator('#imagePlanConfirmed').check();await p.locator('#imageGenerate').click();
    assert.match(await p.locator('#imageFeedback').innerText(),/4000/);assert.equal(f.posts.length,0);
    assert.match(await p.locator('#imagePlanFields .design-character-count').first().innerText(),/4001 \/ 4000/);
    for(let i=0;i<3;i++)await fields.nth(i).fill('字'.repeat(4000));await fields.nth(3).fill('字');
    await p.locator('#imagePlanConfirmed').check();await p.locator('#imageGenerate').click();
    assert.match(await p.locator('#imageFeedback').innerText(),/12000/);assert.equal(f.posts.length,0);
    for(let i=0;i<4;i++)await fields.nth(i).fill('正常内容');
    await p.locator('#imageInstruction').fill('字'.repeat(4001));await p.locator('#imageGenerate').click();
    assert.match(await p.locator('#imageFeedback').innerText(),/4000/);assert.equal(f.posts.length,0);
    await p.locator('#imageInstruction').fill('小鸟\u202e');await p.locator('#imageGenerate').click();
    assert.match(await p.locator('#imageFeedback').innerText(),/控制字符/);assert.equal(f.posts.length,0);
    await p.locator('#imageInstruction').fill('');const normal='<script>alert(1)</script> 中文标点！👨‍👩‍👧‍👦';
    await fields.first().fill(normal);await p.locator('#imagePlanConfirmed').check();
    f.setJob({id:'valid-content',status:'queued'});await p.locator('#imageGenerate').click();
    await p.waitForFunction(()=>document.querySelector('#imageGenerating').classList.contains('hidden')===false);
    assert.equal(f.posts.length,1);assert.equal(f.posts[0].answers[0],normal);assert.equal(await p.locator('#imagePlanFields script').count(),0);
  }finally{await p.close();}
});

test('first generation keeps the thinking panel visible when the provider returns no reasoning', async () => {
  const f = await fixture(), p = f.page;
  try {
    assert.equal(await p.locator('#htmlThinking').isVisible(),false);
    f.setJob({id:'first',status:'queued'});
    await p.locator('#imagePlanConfirmed').check(); await p.locator('#imageGenerate').click();
    await p.locator('#htmlThinking').waitFor({state:'visible'});
    assert.equal(await p.locator('#htmlThinking').evaluate(el=>el.open),true);
    assert.match(await p.locator('#htmlThinkingHint').innerText(),/排队/);
    f.setJob({id:'first',status:'running',generationPhase:'writing',reasoning:''}); await f.tick();
    await p.waitForFunction(()=>document.querySelector('#htmlThinkingHint').textContent.includes('未返回'));
    assert.match(await p.locator('#imageGeneratingTitle').innerText(),/编写网页/);
    assert.equal(await p.locator('#htmlThinkingText').isVisible(),false);
    await p.screenshot({path:'/private/tmp/design-thinking-no-content.png',fullPage:true});
    f.setJob({id:'first',status:'succeeded',generationPhase:'saving',reasoning:''}); await f.tick();
    await p.waitForFunction(()=>document.querySelector('#htmlThinkingStatus').textContent==='生成完成');
    assert.equal(await p.locator('#htmlThinking').evaluate(el=>el.open),false);
    await p.locator('#htmlThinking summary').click();
    assert.match(await p.locator('#htmlThinkingHint').innerText(),/本次模型未返回/);
  } finally { await p.close(); }
});

test('subsequent English reasoning, fallback and Chinese updates stay distinct and preserve manual folding', async () => {
  const english='Fix the bird SVG to look like a cute normal bird. Return full HTML.';
  const f=await fixture('html',{id:'second',status:'running',generationPhase:'writing',reasoning:english}),p=f.page;
  try {
    await p.waitForFunction(()=>document.querySelector('#htmlThinkingText').textContent.includes('Fix the bird'));
    assert.equal(await p.locator('#htmlThinkingText').innerText(),english);
    assert.match(await p.locator('#htmlThinkingHint').innerText(),/英文原文/);
    await p.locator('#htmlThinking summary').click();
    await f.tick(); assert.equal(await p.locator('#htmlThinking').evaluate(el=>el.open),false);
    f.setJob({id:'second',status:'running',generationPhase:'requesting',reasoning:''}); await f.tick();
    await p.waitForFunction(()=>document.querySelector('#htmlThinkingText').textContent==='');
    assert.equal(await p.locator('#htmlThinking').isVisible(),true);
    assert.match(await p.locator('#htmlThinkingHint').textContent(),/等待模型/);
    const chinese='先把小鸟改成圆润的轮廓，再检查按钮交互。';
    f.setJob({id:'second',status:'running',generationPhase:'writing',reasoning:chinese});await f.tick();
    await p.waitForFunction(()=>document.querySelector('#htmlThinkingText').textContent.includes('圆润'));
    assert.equal(await p.locator('#htmlThinking').evaluate(el=>el.open),false);
    await p.locator('#htmlThinking summary').click();assert.equal(await p.locator('#htmlThinkingText').innerText(),chinese);
    assert.equal(await p.locator('#htmlThinkingHint').isVisible(),false);
    await p.setViewportSize({width:390,height:844});await p.screenshot({path:'/private/tmp/design-thinking-chinese-mobile.png',fullPage:true});
    assert.equal(await p.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
  } finally {await p.close();}
});

test('image generation does not display the HTML thinking panel',async()=>{
  const {page:p}=await fixture('image',{id:'picture',status:'running'});
  try {assert.equal(await p.locator('#htmlThinking').isVisible(),false);} finally {await p.close();}
});
