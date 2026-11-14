// Isolated browser transport tests. No production data or model calls are used.
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { resolve, extname } from 'node:path';
import { chromium } from 'playwright';

let browser, server, origin;
before(async () => {
  const root=resolve('frontend');
  server=createServer(async(req,res)=>{
    try{
      const path=resolve(root,'.'+new URL(req.url,'http://local').pathname);
      if(!path.startsWith(root+'/')){res.writeHead(403).end();return;}
      res.setHeader('Content-Type', {'.html':'text/html','.js':'text/javascript','.css':'text/css'}[extname(path)]||'text/plain');
      res.end(await readFile(path));
    }catch{res.writeHead(404).end();}
  });
  await new Promise(done=>server.listen(0,'127.0.0.1',done));
  origin=`http://127.0.0.1:${server.address().port}`;
  browser=await chromium.launch({headless:true});
});
after(async()=>{await browser?.close();await new Promise(done=>server?server.close(done):done());});

async function teacher(){
  const page=await browser.newPage();page.setDefaultTimeout(5000);
  await page.addInitScript(()=>{
    const originalFetch=window.fetch;
    window.exportMode='pending';window.exportRequests=[];
    // A minimal ZIP end record with a comment; completed fixtures need no student content.
    window.zipBytes=()=>{const data=new Uint8Array(25);data.set([80,75,5,6]);data[20]=3;data.set([65,66,67],22);return data;};
    window.fetch=(url,options)=>{
      if(!/\/(images-export|html-export|export)(\?|$)/.test(String(url)))return originalFetch(url,options);
      window.exportRequests.push(String(url));
      if(window.exportMode==='pending')return new Promise(()=>{}); // Deliberately ignores abort.
      if(window.exportMode==='http-error')return Promise.resolve(Response.json({error:'模拟服务器失败'},{status:500}));
      if(window.exportMode==='json-error')return Promise.resolve(Response.json({error:'模拟打包失败'}));
      if(window.exportMode==='empty')return Promise.resolve(new Response('',{headers:{'Content-Type':'application/zip'}}));
      if(window.exportMode==='truncated')return Promise.resolve(new Response('PK unfinished',{headers:{'Content-Type':'application/zip'}}));
      if(window.exportMode==='stream')return Promise.resolve(new Response(new ReadableStream({start(controller){window.exportStream=controller;controller.enqueue(new Uint8Array(1024));}}),{headers:{'Content-Type':'application/zip'}}));
      return Promise.resolve(new Response(window.zipBytes(),{headers:{'Content-Type':'application/zip'}}));
    };
  });
  const plans=[{id:1,title:'模拟图片方案',outputType:'image',questions:['问题'],classIds:[1]}, {id:2,title:'模拟网页方案',outputType:'html',questions:['问题'],classIds:[1]}];
  await page.route('**/api/v2/**',async route=>{
    const path=new URL(route.request().url()).pathname;
    let data={};
    if(path==='/api/v2/design-plans')data={plans};
    else if(path==='/api/v2/classes')data={classes:[{id:1,name:'模拟班级'}]};
    else if(path.endsWith('/records'))data={students:[]};
    else if(path.endsWith('/progress'))data={stage:3};
    else if(path.endsWith('/design-active-plan'))data={plan:plans[0]};
    await route.fulfill({json:{success:true,data}});
  });
  await page.goto(origin+'/teacher.html');
  await page.evaluate(async()=>{
    document.querySelector('#mainContainer').classList.remove('hidden');
    document.querySelector('#passwordOverlay').remove();
    document.querySelectorAll('.tab-content').forEach(n=>n.classList.remove('active'));
    document.querySelector('#design').classList.add('active');
    await(await import('./v2/js/design/teacher.js?v=2026111401')).loadDesignTeacher();
  });
  await page.locator('#dpImageExport').waitFor();return page;
}

const enabled=page=>page.waitForFunction(()=>!document.querySelector('#dpImageExport').disabled&&!document.querySelector('#dpExport').disabled);

test('export: header stall times out and both buttons recover for retry',async()=>{
  const page=await teacher();
  try{
    await page.clock.install();await page.locator('#dpImageExport').click();
    assert.equal(await page.locator('#dpExport').isDisabled(),true);
    assert.match(await page.locator('#dpExportMessage').innerText(),/正在准备/);
    await page.clock.fastForward(60001);await enabled(page);
    assert.match(await page.locator('#dpExportMessage').innerText(),/超时/);
    await page.evaluate(()=>window.exportMode='success');
    const downloaded=page.waitForEvent('download');await page.locator('#dpImageExport').click();
    assert.equal((await downloaded).suggestedFilename(),'方案设计-模拟图片方案.zip');await enabled(page);
    assert.equal(await page.evaluate(()=>window.exportRequests.length),2);
  }finally{await page.close();}
});

test('export: stalled body times out; cancellation also restores buttons',async()=>{
  const page=await teacher();
  try{
    await page.clock.install();await page.evaluate(()=>window.exportMode='stream');
    await page.locator('#dpImageExport').click();
    await page.waitForFunction(()=>document.querySelector('#dpExportMessage').textContent.includes('1.0 KB'));
    await page.clock.fastForward(60001);await enabled(page);
    assert.match(await page.locator('#dpExportMessage').innerText(),/超时/);
    await page.evaluate(()=>window.exportMode='pending');await page.locator('#dpImageExport').click();
    await page.locator('#dpCancelExport').click();await enabled(page);
    assert.match(await page.locator('#dpExportMessage').innerText(),/已取消/);
    assert.equal(await page.locator('#dpCancelExport').isVisible(),false);
  }finally{await page.close();}
});

test('export: progressing transfers may exceed one minute and report received bytes',async()=>{
  const page=await teacher();
  try{
    await page.clock.install();await page.evaluate(()=>window.exportMode='stream');
    await page.locator('#dpClassFilter').selectOption('1');await page.locator('#dpImageExport').click();
    await page.waitForFunction(()=>!!window.exportStream);
    for(let i=0;i<3;i++){
      await page.clock.fastForward(40000);
      await page.evaluate(()=>window.exportStream.enqueue(new Uint8Array(1024)));
      await page.waitForFunction(n=>document.querySelector('#dpExportMessage').textContent.includes(`${n}.0 KB`),i+2);
      assert.equal(await page.locator('#dpImageExport').isDisabled(),true);
    }
    const downloaded=page.waitForEvent('download');
    await page.evaluate(()=>{window.exportStream.enqueue(window.zipBytes());window.exportStream.close();});
    assert.match((await downloaded).suggestedFilename(),/\.zip$/);await enabled(page);
    assert.match(await page.evaluate(()=>window.exportRequests[0]),/images-export\?classId=1$/);
    assert.match(await page.locator('#dpExportMessage').innerText(),/已发起浏览器下载/);
  }finally{await page.close();}
});

test('export: HTTP, JSON, empty and incomplete ZIP errors never report success',async()=>{
  const page=await teacher();
  try{
    let downloads=0;page.on('download',()=>downloads++);
    for(const [mode,message] of [['http-error',/模拟服务器失败/],['json-error',/模拟打包失败/],['empty',/为空/],['truncated',/传输不完整/]]){
      await page.evaluate(mode=>window.exportMode=mode,mode);await page.locator('#dpImageExport').click();await enabled(page);
      assert.match(await page.locator('#dpExportMessage').innerText(),message);
    }
    assert.equal(downloads,0);
  }finally{await page.close();}
});

test('export: switching plans preserves task identity and restores new controls',async()=>{
  const page=await teacher();
  try{
    await page.evaluate(()=>window.exportMode='stream');await page.locator('#dpImageExport').click();
    await page.waitForFunction(()=>!!window.exportStream);await page.locator('#dpSelect').selectOption('2');
    assert.equal(await page.locator('#dpImageExport').isDisabled(),true);
    const downloaded=page.waitForEvent('download');
    await page.evaluate(()=>{window.exportStream.enqueue(window.zipBytes());window.exportStream.close();});
    assert.equal((await downloaded).suggestedFilename(),'方案设计-模拟图片方案.zip');await enabled(page);
    assert.equal(await page.locator('#dpImageExport').innerText(),'导出全部网页 ZIP');
    assert.equal(await page.locator('#dpExportMessage').innerText(),'');
    await page.evaluate(()=>window.exportMode='success');
    for(let i=0;i<2;i++){
      const next=page.waitForEvent('download');await page.locator('#dpImageExport').click();
      assert.equal((await next).suggestedFilename(),'方案设计-模拟网页方案.zip');await enabled(page);
    }
    assert.match(await page.evaluate(()=>window.exportRequests[1]),/html-export$/);
  }finally{await page.close();}
});
