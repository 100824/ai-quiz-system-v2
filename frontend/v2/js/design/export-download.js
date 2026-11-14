// Timeout means no response/data for a minute, not a size or total-duration cap.
// Large ZIP files can keep downloading while data is arriving.
export async function fetchExportBlob(url, {signal,onProgress,idleTimeoutMs=60000}={}) {
  const controller=new AbortController();let timer,reason='已取消导出。',abortListener;
  const abort=message=>{reason=message;controller.abort();};
  const cancel=()=>abort('已取消导出。');
  const touch=()=>{clearTimeout(timer);timer=setTimeout(()=>abort('导出等待超时，超过60秒未收到数据，请重试或缩小班级范围。'),idleTimeoutMs);};
  const interrupted=new Promise((_,reject)=>{abortListener=()=>reject(new Error(reason));controller.signal.addEventListener('abort',abortListener,{once:true});});
  if(signal?.aborted)cancel();else signal?.addEventListener('abort',cancel,{once:true});
  const receive=async()=>{
    if(controller.signal.aborted)throw new Error(reason);
    touch();
    const response=await fetch(url,{signal:controller.signal,cache:'no-store'});touch();
    if(!response.ok){const data=await response.json().catch(()=>({}));throw new Error(data.error||`导出请求失败（HTTP ${response.status}）`);}
    const type=response.headers.get('content-type')||'';
    if(/application\/json|text\/html/i.test(type)){const data=/json/i.test(type)?await response.json().catch(()=>({})):{};throw new Error(data.error||'服务器没有返回导出文件，请重试。');}
    const length=Number(response.headers.get('content-length'));
    const total=Number.isFinite(length)&&length>0?length:null;
    let received=0;
    const progress=()=>{if(!controller.signal.aborted)onProgress?.({received,total});};progress();
    let blob;
    if(response.body && typeof TransformStream!=='undefined'){
      const body=response.body.pipeThrough(new TransformStream({transform(chunk,out){touch();received+=chunk.byteLength;progress();out.enqueue(chunk);}}),{signal:controller.signal});
      blob=await new Response(body,{headers:{'Content-Type':type||'application/octet-stream'}}).blob();
    }else{blob=await response.blob();received=blob.size;touch();progress();}
    if(!blob.size)throw new Error('导出文件为空，请重试。');
    if(total && blob.size!==total)throw new Error('导出文件传输不完整，请重试。');
    // A streaming ZIP can be cut short by the server after it already sent HTTP 200.
    // Check its end record before reporting success; this reads at most 64 KiB.
    if(/application\/(?:x-)?zip/i.test(type)){
      const tail=new DataView(await blob.slice(-65557).arrayBuffer());let complete=false;
      for(let i=tail.byteLength-22;i>=0;i--){
        if(tail.getUint32(i,true)===0x06054b50 && i+22+tail.getUint16(i+20,true)===tail.byteLength){complete=true;break;}
      }
      if(!complete)throw new Error('ZIP 文件传输不完整，请重试。');
    }
    return blob;
  };
  try{return await Promise.race([receive(),interrupted]);}
  finally{clearTimeout(timer);signal?.removeEventListener('abort',cancel);controller.signal.removeEventListener('abort',abortListener);controller.abort();}
}

export function exportBytes(bytes){return bytes<1024*1024?`${(bytes/1024).toFixed(1)} KB`:`${(bytes/(1024*1024)).toFixed(1)} MB`;}
