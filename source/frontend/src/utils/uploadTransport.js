import { globalVars } from '@/utils/constants';
import { getApiPath } from '@/utils/url.js';
import { renew } from '@/utils/auth';

let probe;
let preferFallbackUntil = 0;
let probeExpires = 0;
const tickets = new Map();

async function directAvailable() {
  const health = globalVars.hwsubmitCourse?.uploadHealthURL;
  if (!health || Date.now() < preferFallbackUntil) return false;
  if (Date.now() >= probeExpires) {
    probeExpires = Date.now() + 30000;
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 1500);
    probe = fetch(health, { credentials:'omit', cache:'no-store', signal:controller.signal })
      .then(r => r.ok).catch(() => false).finally(() => clearTimeout(timer));
  }
  return probe;
}

function uploadError(status, body) {
  let message = body;
  try { message = JSON.parse(body).message || body; } catch { /* plain response */ }
  const error = new Error(message || (status ? `上传失败 (${status})` : '网络连接中断'));
  error.response = { status, responseText:body };
  return error;
}

// The operation owns the active XHR across ticket lookup, retries and fallback.
// Aborting it prevents a pending retry from restarting a paused/cancelled file.
export function postUploadChunk({ source, path, body, overwrite=false, headers, onprogress }) {
  const operation = { aborted:false, xhr:null, controller:new AbortController() };
  const session = headers['X-File-Upload-Session'];
  const total = headers['X-File-Total-Size'];
  const key = `${source}\0${path}\0${session}\0${overwrite}\0${total}`;
  const checkAbort = () => { if (operation.aborted) throw new Error('Upload aborted'); };

  const send = (url, direct, token, part=body, partHeaders=headers, progress=onprogress) => new Promise((resolve,reject) => {
    checkAbort();
    const xhr = new XMLHttpRequest();
    operation.xhr = xhr;
    let idleTimer;
    const touch = () => {
      clearTimeout(idleTimer);
      idleTimer = setTimeout(() => { xhr.abort(); resolve({status:0,body:''}); },45000);
    };
    const finish = (status,response) => { clearTimeout(idleTimer); resolve({status,body:response}); };
    xhr.open('POST',url,true);
    xhr.timeout = 300000;
    xhr.withCredentials = !direct;
    xhr.setRequestHeader('Content-Type','application/octet-stream');
    for (const [name,value] of Object.entries(partHeaders)) xhr.setRequestHeader(name,String(value));
    if (direct) xhr.setRequestHeader('X-Hwsubmit-Upload-Token',token);
    xhr.upload.onprogress = event => {
      touch();
      if (event.lengthComputable) progress?.(100*event.loaded/event.total);
    };
    xhr.onload = () => finish(xhr.status,xhr.responseText);
    xhr.onerror = () => finish(0,'');
    xhr.ontimeout = () => finish(0,'');
    xhr.onabort = () => {
      clearTimeout(idleTimer);
      if (operation.aborted) reject(new Error('Upload aborted'));
      else resolve({status:0,body:''});
    };
    touch();
    xhr.send(part);
  });

  const promise = (async () => {
    let useDirect = await directAvailable();
    checkAbort();
    let ticket;
    if (useDirect) {
      ticket = tickets.get(key);
      if (!ticket || ticket.expires < Date.now()+30000) {
        const ticketTimer = setTimeout(() => operation.controller.abort(),10000);
        try {
          const response = await fetch(getApiPath('hwsubmit/upload-ticket'), {
            method:'POST', credentials:'same-origin',
            headers:{'Content-Type':'application/json'}, signal:operation.controller.signal,
            body:JSON.stringify({source,path,session,total:Number(total),override:overwrite})
          });
          if (!response.ok) throw new Error('upload ticket unavailable');
          ticket = await response.json();
          for (const [id,value] of tickets) if (value.expires < Date.now()) tickets.delete(id);
          tickets.set(key,ticket);
        } catch { useDirect = false; } finally { clearTimeout(ticketTimer); }
      }
    }
    const fallbackURL = getApiPath('resources',{source,path,override:overwrite});
    let response;
    const succeeded = result => result.status >= 200 && result.status < 300;
    const retryable = status => status === 0 || status === 408 || status === 429 || status >= 500;
    const complete = result => {
      if (Number(headers['X-File-Chunk-Offset'] || 0)+body.size >= Number(total)) tickets.delete(key);
      return result.body;
    };
    if (useDirect) {
      response = await send(ticket.url,true,ticket.token);
      checkAbort();
      if (succeeded(response)) return complete(response);
      if (!retryable(response.status) && response.status !== 401) throw uploadError(response.status,response.body);
      preferFallbackUntil = Date.now()+30000;
      probe = Promise.resolve(false);
      probeExpires = preferFallbackUntil;
    }
    // Slow CF-to-origin links acknowledge smaller pieces promptly; a 16 MiB
    // request could otherwise wait past the browser's stall timeout.
    const baseOffset = Number(headers['X-File-Chunk-Offset'] || 0);
    for (let offset=0;offset<body.size;offset+=2*1024*1024) {
      const part = body.slice(offset,offset+2*1024*1024);
      const partHeaders = {...headers,'X-File-Chunk-Offset':baseOffset+offset};
      const progress = percent => onprogress?.((offset+part.size*percent/100)*100/body.size);
      let renewed = false;
      for (let attempt=0;attempt<4;attempt++) {
        checkAbort();
        response = await send(fallbackURL,false,null,part,partHeaders,progress);
        checkAbort();
        if (succeeded(response)) break;
        if (response.status === 401 && !renewed) {
          renewed = true;
          await renew();
          continue;
        }
        if (!retryable(response.status)) break;
        await new Promise(resolve => setTimeout(resolve,Math.min(2000,500*2**attempt)));
      }
      if (!succeeded(response)) throw uploadError(response.status,response.body);
      // A retry after a lost final response may confirm the whole file already exists.
      try { if (JSON.parse(response.body).complete) return complete(response); } catch { /* older server */ }
    }
    return complete(response);
  })();
  Object.defineProperty(promise,'xhr',{get:()=>operation.xhr});
  promise.abort = () => {
    operation.aborted = true;
    operation.controller.abort();
    operation.xhr?.abort();
  };
  return promise;
}
