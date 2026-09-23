const $=(s,r=document)=>r.querySelector(s);
const $$=(s,r=document)=>[...r.querySelectorAll(s)];
const PAGES={overview:"概览",accounts:"账号",models:"模型",api:"API"};
let currentPage="overview";
let accountCache=[];
let keysCache=[];
let apiTestModels=[];
let apiTestController=null;
let apiTestRows=new Map();
let apiTestBatchStarted=0;
let apiTestRenderPending=false;

async function api(path,opts={}){
  const init={credentials:"same-origin",...opts};
  if(opts.body!==undefined){
    init.headers={"Content-Type":"application/json",...(opts.headers||{})};
    init.body=JSON.stringify(opts.body);
  }
  const res=await fetch("/api"+path,init);
  if(res.status===401){location.replace("/login");throw new Error("会话已过期");}
  let data=null;try{data=await res.json();}catch{}
  if(!res.ok)throw new Error(data?.error||data?.error?.message||("HTTP "+res.status));
  return data;
}
const esc=(v)=>String(v??"").replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));
const icon=(name,cls="ui-icon")=>'<svg class="'+cls+'" aria-hidden="true"><use href="#i-'+name+'"></use></svg>';
function toast(msg,tone="info"){const root=$("#toast-root"),el=document.createElement("div");el.className="toast "+tone;el.textContent=msg;root.append(el);setTimeout(()=>el.remove(),3000);}
function fmtTime(sec){if(!sec)return "—";const d=new Date(sec*1000);return Number.isNaN(d.getTime())?"—":d.toLocaleString("zh-CN",{hour12:false});}
function badge(text,tone=""){return '<span class="badge '+tone+'">'+esc(text)+"</span>";}
function stateBadge(state){return state==="healthy"?badge("可用","good"):state==="cooling_down"?badge("冷却","warn"):state==="suspended"?badge("已风控","bad"):state==="disabled"?badge("已停用",""):badge(state||"未知","");}
function openDialog(id){const d=$("#"+id);if(d&&!d.open)d.showModal();}
function closeDialog(id){const d=$("#"+id);if(d?.open)d.close();}

function setTheme(theme){
  document.documentElement.dataset.theme=theme;
  localStorage.setItem("verdent-theme",theme);
  const action=$("#theme-action");
  if(action){
    const dark=theme==="dark";
    action.innerHTML=icon(dark?"sun":"moon")+"<span>"+(dark?"切换浅色模式":"切换深色模式")+"</span>";
  }
}
function initTheme(){setTheme(localStorage.getItem("verdent-theme")||"light");}
function closeTopMore(){const d=$("#top-more");if(d?.open)d.removeAttribute("open");}
function setPage(page){
  currentPage=page;
  $$(".page").forEach(x=>x.hidden=x.id!=="page-"+page);
  $("#page-title").textContent=PAGES[page]||page;
  closeTopMore();
  const drawer=$("#mobile-drawer");if(drawer?.open)drawer.close();
  loadPage(page).catch(e=>toast(e.message,"bad"));
}
async function loadPage(page){
  if(page==="overview")return loadOverview();
  if(page==="accounts")return loadAccounts();
  if(page==="models")return loadModels();
  if(page==="api")return loadAPI();
}

async function loadOverview(){
  const d=await api("/overview");
  $("#version").textContent=d.version||"dev";
  const a=d.accounts||{},total=Number(a.total||0);
  const cards=[
    {k:"账号总数",n:total,sub:"持久化账号池"},
    {k:"健康账号",n:a.healthy||0,sub:"可参与路由",cls:(a.healthy||0)>0?"good":"warn"},
    {k:"冷却 / 风控",n:Number(a.cooling_down||0)+Number(a.suspended||0),sub:(a.suspended||0)+" suspended · "+(a.cooling_down||0)+" cooling",cls:(a.suspended||0)?"warn":""},
    {k:"兼容接口",n:"3",sub:"Chat · Responses · Models",cls:"good"},
  ];
  $("#kpi").innerHTML=cards.map(c=>'<div class="kpi-card '+(c.cls||"")+'"><div class="k">'+esc(c.k)+'</div><div class="n">'+esc(c.n)+'</div><div class="sub">'+esc(c.sub)+'</div></div>').join("");
  const rows=[
    ["健康",a.healthy||0,"good"],["冷却",a.cooling_down||0,"warn"],["已风控",a.suspended||0,"bad"],["已停用",a.disabled||0,""]
  ];
  $("#account-summary").innerHTML='<div class="summary-bars">'+rows.map(([name,n,tone])=>{
    const pct=total?Math.round(Number(n)/total*100):0;
    return '<div class="summary-line"><span>'+esc(name)+'</span><div class="bar '+tone+'"><i style="width:'+pct+'%"></i></div><b>'+n+'</b></div>';
  }).join("")+"</div>";
  const p=d.protocol||{};
  $("#runtime").innerHTML=[
    ["Verdent 版本",p.app_version||"—"],["协议 Beta",p.beta||"—"],["上游 Endpoint",p.endpoint||"—"],["Chat Completions","Ready"],["Responses API","Ready"]
  ].map(([k,v])=>'<div class="runtime-row"><span>'+esc(k)+'</span><span class="'+(String(v).startsWith("http")?"mono wrap-anywhere":"")+'">'+esc(v)+'</span></div>').join("");
}

async function loadAccounts(){
  const d=await api("/accounts");
  accountCache=d.accounts||[];
  $("#accounts-meta").textContent=accountCache.length?accountCache.length+" 个账号 · "+(d.store||"managed store"):"账号池为空，可从上方添加。";
  if(!accountCache.length){$("#accounts-table").innerHTML='<div class="empty">还没有账号。推荐先使用“浏览器登录”。</div>';return;}
  $("#accounts-table").innerHTML='<div class="table-wrap"><table class="data-table responsive-table"><thead><tr><th>账号</th><th>启用</th><th>状态</th><th>Token</th><th>Device / Team</th><th>固定代理</th><th>操作</th></tr></thead><tbody>'+accountCache.map(a=>{
    const expiry=a.token_expires?fmtTime(a.token_expires):"未知";
    const proxy=a.proxy_url||"直连";
    const enabled=a.enabled!==false;
    const runtime=a.runtime_state||a.state;
    return '<tr class="'+(enabled?"":"account-row-disabled")+'">'+
      '<td data-label="账号"><div class="cell-main">'+esc(a.label||a.id)+'</div><div class="cell-sub mono">'+esc(a.id)+'</div></td>'+
      '<td data-label="启用"><label class="account-switch" title="'+(enabled?"点击停用该账号":"点击启用该账号")+'"><input type="checkbox" data-enabled="'+esc(a.id)+'" '+(enabled?"checked":"")+'><span class="switch-track"><i></i></span><b>'+(enabled?"启用":"停用")+'</b></label></td>'+
      '<td data-label="状态">'+stateBadge(a.state)+(a.state==="disabled"&&runtime!=="healthy"?'<div class="cell-sub">底层 '+esc(runtime)+'</div>':"")+(a.last_error?'<div class="cell-sub wrap-anywhere">'+esc(a.last_error.slice(0,100))+'</div>':"")+'</td>'+
      '<td data-label="Token"><div class="cell-main">'+esc(a.token_uid||"—")+'</div><div class="cell-sub">过期 '+esc(expiry)+'</div></td>'+
      '<td data-label="设备"><div class="mono wrap-anywhere">'+esc(a.device_id||"—")+'</div><div class="cell-sub">Team '+esc(a.team_id||"0")+'</div></td>'+
      '<td data-label="固定代理"><div class="mono wrap-anywhere">'+esc(proxy)+'</div></td>'+
      '<td data-label="操作"><div class="table-actions">'+
      '<button class="table-action" data-test="'+esc(a.id)+'">'+icon("check","mini-icon")+'<span>测试</span></button>'+
      '<button class="table-action" data-proxy="'+esc(a.id)+'">'+icon("link","mini-icon")+'<span>代理</span></button>'+
      '<button class="table-action danger" data-delete="'+esc(a.id)+'">'+icon("trash","mini-icon")+'<span>删除</span></button>'+
      '</div></td></tr>';
  }).join("")+"</tbody></table></div>";
  $("[data-test]").forEach(b=>b.onclick=()=>testAccount(b.dataset.test,b));
  $("[data-proxy]").forEach(b=>b.onclick=()=>openProxy(b.dataset.proxy));
  $("[data-enabled]").forEach(b=>b.onchange=()=>setAccountEnabled(b.dataset.enabled,b.checked,b));
  $("[data-delete]").forEach(b=>b.onclick=()=>deleteAccount(b.dataset.delete));
}

function diagnosticSummary(r){
  const d=r.diagnostics||{},parts=[];
  const mark=s=>s==="ok"?"✓":s==="skipped"?"—":s==="suspended"?"✕":s==="cooling_down"?"!":"✕";
  if(d.credential)parts.push("凭据 "+mark(d.credential.status));
  if(d.catalog)parts.push("模型目录 "+mark(d.catalog.status));
  if(d.free_inference)parts.push("Free inference "+mark(d.free_inference.status));
  return parts.join(" · ");
}
async function testAccount(id,button){
  button.disabled=true;
  try{
    const r=await api("/accounts/"+encodeURIComponent(id)+"/test",{method:"POST"});
    const free=r.diagnostics?.free_inference;
    const catalog=r.diagnostics?.catalog;
    if(r.state==="suspended"||free?.status==="suspended"||catalog?.status==="suspended"){
      toast(diagnosticSummary(r)+" · 80006 已风控","bad");
    }else if(r.ok){
      toast(diagnosticSummary(r)+" · "+r.models+" 个模型","good");
    }else{
      toast(diagnosticSummary(r)+" · "+(free?.detail||catalog?.detail||"诊断失败"),"bad");
    }
    await loadAccounts();await loadOverview();
  }
  catch(e){toast(e.message,"bad");}
  finally{button.disabled=false;}
}
async function setAccountEnabled(id,enabled,input){
  if(input)input.disabled=true;
  try{
    await api("/accounts/"+encodeURIComponent(id)+"/state",{method:"POST",body:{disabled:!enabled}});
    toast(enabled?"账号已启用并允许参与路由":"账号已停用，不再参与路由","good");
    await loadAccounts();await loadOverview();
  }catch(e){
    if(input)input.checked=!enabled;
    toast(e.message,"bad");
  }finally{
    if(input)input.disabled=false;
  }
}
async function deleteAccount(id){
  const a=accountCache.find(x=>x.id===id);if(!confirm("确认删除 "+(a?.label||id)+"？\nToken 将从持久化账号文件中移除。"))return;
  await api("/accounts/"+encodeURIComponent(id),{method:"DELETE"});toast("账号已删除","good");await loadAccounts();await loadOverview();
}
function openProxy(id){
  const a=accountCache.find(x=>x.id===id);$("#proxy-id").value=id;$("#proxy-account").textContent=(a?.label||id)+" · 当前 "+(a?.proxy_url||"直连");$("#proxy-value").value="";$("#proxy-error").textContent="";openDialog("proxy-modal");
}
async function saveProxy(ev){
  ev.preventDefault();const id=$("#proxy-id").value,raw=$("#proxy-value").value.trim();$("#proxy-error").textContent="";
  try{await api("/accounts/"+encodeURIComponent(id)+"/proxy",{method:"POST",body:{proxy_url:raw}});closeDialog("proxy-modal");toast(raw?"固定代理已更新":"已切换为直连","good");await loadAccounts();}
  catch(e){$("#proxy-error").textContent=e.message;}
}
async function saveToken(ev){
  ev.preventDefault();$("#token-error").textContent="";
  const body={token:$("#token-value").value.trim(),label:$("#token-label").value.trim(),proxy_url:$("#token-proxy").value.trim(),device_id:$("#token-device").value.trim(),team_id:$("#token-team").value.trim()||"0"};
  try{
    const r=await api("/accounts/token",{method:"POST",body});closeDialog("token-modal");ev.target.reset();$("#token-team").value="0";toast("账号 "+(r.account?.label||r.account?.id||"")+" 已保存","good");await loadAccounts();await loadOverview();
  }catch(e){$("#token-error").textContent=e.message;}
}

async function browserLogin(){
  const popup=window.open("about:blank","verdent-auth","width=860,height=760");
  try{
    const start=await api("/auth/verdent/start",{method:"POST"});
    if(popup)popup.location.href=start.auth_url;else window.open(start.auth_url,"_blank");
    toast("已打开 Verdent 登录页");
    pollOAuth(start.flow_id);
  }catch(e){if(popup)popup.close();toast(e.message,"bad");}
}
async function pollOAuth(flowID){
  const until=Date.now()+5*60*1000;
  while(Date.now()<until){
    await new Promise(r=>setTimeout(r,1000));
    try{
      const s=await api("/auth/verdent/status?flow_id="+encodeURIComponent(flowID));
      if(s.status==="complete"){toast("Verdent 账号登录成功","good");await loadAccounts();await loadOverview();return;}
      if(s.status==="failed"){toast(s.error||"Verdent 登录失败","bad");return;}
    }catch(e){if(!String(e.message).includes("not found"))console.warn(e);}
  }
  toast("Verdent 登录等待超时","bad");
}
async function desktopImport(){
  const b=$("#desktop-import");b.disabled=true;
  try{const r=await api("/auth/verdent/desktop",{method:"POST"});toast("已导入 "+(r.account?.label||r.account?.id||"Verdent 账号"),"good");await loadAccounts();await loadOverview();}
  catch(e){toast(e.message,"bad");}
  finally{b.disabled=false;}
}

async function loadModels(){
  $("#models-meta").textContent="正在读取 Verdent 模型目录…";
  try{
    const d=await api("/models"),models=d.models||[];
    $("#models-meta").textContent=models.length?models.length+" 个模型 · 账号 "+(d.account_id||"自动"):"暂无模型";
    $("#models-table").innerHTML=models.length?'<div class="table-wrap"><table class="data-table responsive-table"><thead><tr><th>模型</th><th>Family</th><th>上下文</th><th>能力</th><th>额度</th></tr></thead><tbody>'+models.map(m=>{
      const ctx=(m.context_windows||[]).map(x=>x.display).join(" / ")||"—";
      const caps=[m.supports_thinking?"Thinking":"",m.supports_images?"Image":""].filter(Boolean).join(" · ")||"Text";
      return '<tr><td data-label="模型"><div class="cell-main mono wrap-anywhere">'+esc(m.id)+'</div><div class="cell-sub">'+esc(m.name||"")+'</div></td><td data-label="Family">'+esc(m.family||"—")+'</td><td data-label="上下文">'+esc(ctx)+'</td><td data-label="能力">'+esc(caps)+'</td><td data-label="额度">'+(m.is_limit_free?badge("Free","good"):badge("Standard"))+'</td></tr>';
    }).join("")+"</tbody></table></div>":'<div class="empty">没有可显示的模型。</div>';
  }catch(e){$("#models-meta").textContent="读取失败";$("#models-table").innerHTML='<div class="empty">'+esc(e.message)+'</div>';}
}
async function loadAPI(){$("#base-url").textContent=location.origin+"/v1";await Promise.all([loadKeys(),loadAPITestModels()]);}

async function loadKeys(){
  const d=await api("/keys");
  keysCache=d.keys||[];
  $("#keys-meta").textContent=keysCache.length?keysCache.length+" 个密钥 · "+(d.store_file||""):"还没有密钥。";
  if(!keysCache.length){$("#keys-table").innerHTML='<div class="empty">还没有密钥。点击「创建密钥」即可添加。</div>';return;}
  $("#keys-table").innerHTML='<div class="table-wrap"><table class="data-table responsive-table"><thead><tr><th>名称</th><th>密钥</th><th>创建时间</th><th>操作</th></tr></thead><tbody>'+keysCache.map(k=>{
    return '<tr>'+
      '<td data-label="名称"><div class="cell-main">'+esc(k.name||"—")+'</div>'+(k.builtin?'<div class="cell-sub">内置 · 环境变量</div>':'')+'</td>'+
      '<td data-label="密钥"><div class="mono wrap-anywhere">'+esc(k.key)+'</div></td>'+
      '<td data-label="创建时间">'+esc(k.created_at?fmtTime(k.created_at):"—")+'</td>'+
      '<td data-label="操作"><div class="table-actions">'+
      '<button class="table-action" data-copy-key="'+esc(k.id)+'">'+icon("copy","mini-icon")+'<span>复制</span></button>'+
      (k.builtin?'':'<button class="table-action danger" data-del-key="'+esc(k.id)+'">'+icon("trash","mini-icon")+'<span>删除</span></button>')+
      '</div></td></tr>';
  }).join("")+"</tbody></table></div>";
  $$("[data-copy-key]").forEach(b=>b.onclick=()=>copyKey(b.dataset.copyKey));
  $$("[data-del-key]").forEach(b=>b.onclick=()=>deleteKey(b.dataset.delKey));
}
async function copyKey(id){
  const k=keysCache.find(x=>x.id===id);if(!k)return;
  try{await navigator.clipboard.writeText(k.key);toast("密钥已复制","good");}
  catch{toast("复制失败，请手动选择","bad");}
}
async function createKey(ev){
  ev.preventDefault();$("#key-error").textContent="";
  try{
    const r=await api("/keys",{method:"POST",body:{name:$("#key-name").value.trim()}});
    closeDialog("key-modal");ev.target.reset();
    toast("密钥已创建，可在列表中复制","good");await loadKeys();
  }catch(e){$("#key-error").textContent=e.message;}
}
async function deleteKey(id){
  const k=keysCache.find(x=>x.id===id);if(!k)return;
  if(!confirm("确认删除密钥 "+(k.name||id)+"？\n使用该密钥的客户端将立即失效。"))return;
  try{await api("/keys/"+encodeURIComponent(id),{method:"DELETE"});toast("密钥已删除","good");await loadKeys();}
  catch(e){toast(e.message,"bad");}
}


async function loadAPITestModels(){
  const select=$("#api-test-model");
  try{
    const d=await api("/models");
    apiTestModels=d.models||[];
    const current=select.value;
    select.innerHTML=apiTestModels.length?apiTestModels.map(m=>'<option value="'+esc(m.id)+'">'+esc(m.name||m.id)+' · '+esc(m.id)+'</option>').join(""):'<option value="">暂无可用模型</option>';
    if(current&&apiTestModels.some(m=>m.id===current))select.value=current;
    syncAPITestModelLimits();
  }catch(e){
    apiTestModels=[];
    select.innerHTML='<option value="">模型读取失败</option>';
    $("#api-test-context-hint").textContent=e.message;
  }
}
function syncAPITestModelLimits(){
  const model=apiTestModels.find(m=>m.id===$("#api-test-model").value);
  const windows=model?.context_windows||[];
  const list=$("#api-test-context-options");
  list.innerHTML=windows.map(w=>'<option value="'+Number(w.tokens||0)+'">'+esc(w.display||w.tokens)+'</option>').join("");
  const maxContext=windows.reduce((n,w)=>Math.max(n,Number(w.tokens||0)),0);
  const ctx=$("#api-test-context");
  ctx.max=String(Math.max(2000000,maxContext||0));
  $("#api-test-context-hint").textContent=windows.length?"可选："+windows.map(w=>w.display||w.tokens).join(" / ")+"；0 表示模型默认。":"0 表示使用模型默认上下文窗口。";
  const out=$("#api-test-output-tokens");
  if(model?.max_output_tokens>0){
    out.max=String(model.max_output_tokens);
    $("#api-test-output-hint").textContent="该模型目录标注最大输出 "+model.max_output_tokens+" tokens。";
    if(Number(out.value)>model.max_output_tokens)out.value=String(model.max_output_tokens);
  }else{
    out.max="64000";
    $("#api-test-output-hint").textContent="用于控制单请求最大生成长度。";
  }
}
function fmtMS(ms){return Number.isFinite(Number(ms))&&Number(ms)>=0?Number(ms).toLocaleString("zh-CN",{maximumFractionDigits:0})+" ms":"—";}
function fmtTPS(v,estimated=false){
  const n=Number(v||0);if(!n)return "—";
  return (estimated?"≈":"")+n.toFixed(n>=100?1:2)+" tok/s";
}
function benchmarkStatus(row){
  if(row.status==="done")return badge("完成","good");
  if(row.status==="error")return badge("失败","bad");
  if(row.status==="cancelled")return badge("已取消","warn");
  if(row.status==="running")return badge("运行中","warn");
  return badge("等待");
}
function initAPITestRows(count){
  apiTestRows=new Map();
  for(let i=1;i<=count;i++)apiTestRows.set(i,{index:i,status:"queued",ttft_ms:null,elapsed_ms:0,duration_ms:null,throughput_tps:0,throughput_estimated:true,input_tokens:0,output_tokens:0,total_tokens:0,preview:"",error:""});
  apiTestBatchStarted=performance.now();
  $("#api-test-summary").hidden=false;
  scheduleAPITestRender();
}
function scheduleAPITestRender(){
  if(apiTestRenderPending)return;
  apiTestRenderPending=true;
  requestAnimationFrame(()=>{apiTestRenderPending=false;renderAPITest();});
}
function renderAPITest(){
  const rows=[...apiTestRows.values()];
  if(!rows.length)return;
  const done=rows.filter(r=>r.status==="done"),failed=rows.filter(r=>r.status==="error"||r.status==="cancelled");
  const completed=done.length+failed.length;
  const input=done.reduce((n,r)=>n+Number(r.input_tokens||0),0);
  const output=done.reduce((n,r)=>n+Number(r.output_tokens||0),0);
  const total=done.reduce((n,r)=>n+Number(r.total_tokens||0),0);
  const ttfts=done.map(r=>Number(r.ttft_ms)).filter(Number.isFinite);
  const avgTTFT=ttfts.length?ttfts.reduce((a,b)=>a+b,0)/ttfts.length:null;
  const elapsed=Math.max(.001,(performance.now()-apiTestBatchStarted)/1000);
  const batchTPS=output/elapsed;
  $("#api-test-summary").innerHTML=[
    ["进度",completed+" / "+rows.length],
    ["成功 / 失败",done.length+" / "+failed.length],
    ["平均 TTFT",avgTTFT==null?"—":fmtMS(avgTTFT)],
    ["输入 tokens",input.toLocaleString()],
    ["输出 tokens",output.toLocaleString()],
    ["总计 tokens",total.toLocaleString()],
    ["批次吞吐",output?batchTPS.toFixed(2)+" tok/s":"—"],
  ].map(([k,v])=>'<div class="benchmark-kpi"><span>'+esc(k)+'</span><strong>'+esc(v)+'</strong></div>').join("");
  $("#api-test-results").innerHTML='<div class="table-wrap"><table class="data-table benchmark-table"><thead><tr><th>#</th><th>状态</th><th>TTFT</th><th>实时吞吐率</th><th>输入 tokens</th><th>输出 tokens</th><th>总计 tokens</th><th>总耗时</th><th>回复预览</th></tr></thead><tbody>'+
    rows.map(r=>'<tr>'+
      '<td>'+r.index+'</td>'+
      '<td>'+benchmarkStatus(r)+(r.error?'<div class="cell-sub benchmark-error">'+esc(r.error)+'</div>':"")+'</td>'+
      '<td class="mono">'+(r.ttft_ms==null?"—":fmtMS(r.ttft_ms))+'</td>'+
      '<td class="mono">'+fmtTPS(r.throughput_tps,r.throughput_estimated)+'</td>'+
      '<td class="mono">'+(r.input_tokens?Number(r.input_tokens).toLocaleString():"—")+'</td>'+
      '<td class="mono">'+(r.output_tokens?Number(r.output_tokens).toLocaleString():"—")+'</td>'+
      '<td class="mono">'+(r.total_tokens?Number(r.total_tokens).toLocaleString():"—")+'</td>'+
      '<td class="mono">'+((r.duration_ms??r.elapsed_ms)>0?fmtMS(r.duration_ms??r.elapsed_ms):"—")+'</td>'+
      '<td><div class="benchmark-preview">'+esc(r.preview||"—")+'</div></td>'+
    '</tr>').join("")+'</tbody></table></div>';
}
function handleAPITestEvent(event){
  if(event.type!=="request")return;
  const row=apiTestRows.get(Number(event.index));
  if(!row)return;
  Object.assign(row,event);
  if(event.status==="done"||event.status==="error")row.duration_ms=Number(event.duration_ms||event.elapsed_ms||0);
  scheduleAPITestRender();
}
async function consumeAPITestStream(res){
  if(!res.body)throw new Error("浏览器不支持流式响应");
  const reader=res.body.getReader(),decoder=new TextDecoder();
  let buffer="";
  while(true){
    const {value,done}=await reader.read();
    if(done)break;
    buffer+=decoder.decode(value,{stream:true});
    buffer=buffer.replace(/\r\n/g,"\n");
    let split;
    while((split=buffer.indexOf("\n\n"))>=0){
      const block=buffer.slice(0,split);buffer=buffer.slice(split+2);
      const data=block.split("\n").filter(line=>line.startsWith("data:")).map(line=>line.slice(5).trim()).join("\n");
      if(!data)continue;
      try{handleAPITestEvent(JSON.parse(data));}catch(e){console.warn("benchmark event parse failed",e);}
    }
  }
}
function stopAPITest(){
  if(!apiTestController)return;
  apiTestController.abort();
  apiTestRows.forEach(row=>{if(row.status==="queued"||row.status==="running")row.status="cancelled";});
  scheduleAPITestRender();
}
async function runAPITest(ev){
  ev.preventDefault();
  if(apiTestController)return;
  const form=ev.currentTarget;if(!form.reportValidity())return;
  const body={
    model:$("#api-test-model").value,
    prompt:$("#api-test-prompt").value.trim(),
    context_window_tokens:Number($("#api-test-context").value||0),
    concurrency:Number($("#api-test-concurrency").value||1),
    max_output_tokens:Number($("#api-test-output-tokens").value||512),
  };
  if(!body.model){toast("请选择测试模型","bad");return;}
  initAPITestRows(body.concurrency);
  apiTestController=new AbortController();
  $("#api-test-start").disabled=true;$("#api-test-stop").disabled=false;
  $("#api-test-note").textContent="测试进行中：服务端正在发起 "+body.concurrency+" 路并发请求…";
  try{
    const res=await fetch("/api/benchmark/chat",{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:JSON.stringify(body),signal:apiTestController.signal});
    if(res.status===401){location.replace("/login");return;}
    if(!res.ok){
      let data=null;try{data=await res.json();}catch{}
      throw new Error(data?.error||("HTTP "+res.status));
    }
    await consumeAPITestStream(res);
    $("#api-test-note").textContent="测试完成。运行中带 ≈ 的吞吐率为估算值；最终有 usage 时显示精确平均 tokens/s。";
  }catch(e){
    if(e.name==="AbortError"){
      $("#api-test-note").textContent="测试已停止。";
    }else{
      toast(e.message,"bad");
      $("#api-test-note").textContent="测试失败："+e.message;
      apiTestRows.forEach(row=>{if(row.status==="queued"||row.status==="running"){row.status="error";row.error=e.message;}});
    }
  }finally{
    apiTestController=null;
    $("#api-test-start").disabled=false;$("#api-test-stop").disabled=true;
    scheduleAPITestRender();
  }
}

async function logout(){try{await api("/auth/logout",{method:"POST"});}catch{}location.replace("/login");}
function bind(){
  $$("[data-page]").forEach(b=>b.onclick=()=>setPage(b.dataset.page));
  $("#overview-refresh").onclick=()=>loadOverview().catch(e=>toast(e.message,"bad"));
  $("#accounts-refresh").onclick=()=>loadAccounts().catch(e=>toast(e.message,"bad"));
  $("#keys-refresh").onclick=()=>loadKeys().catch(e=>toast(e.message,"bad"));
  $("#theme-action").onclick=()=>{setTheme(document.documentElement.dataset.theme==="dark"?"light":"dark");closeTopMore();};
  $("#logout").onclick=logout;
  $("#top-logout").onclick=logout;
  $("#mobile-menu").onclick=()=>$("#mobile-drawer").showModal();
  $("#drawer-close").onclick=()=>$("#mobile-drawer").close();
  $("#browser-login").onclick=browserLogin;$("#desktop-import").onclick=desktopImport;$("#token-add").onclick=()=>openDialog("token-modal");
  $("#token-form").onsubmit=saveToken;$("#proxy-form").onsubmit=saveProxy;$("#models-refresh").onclick=loadModels;
  $("#key-add").onclick=()=>openDialog("key-modal");$("#key-form").onsubmit=createKey;
  $("#api-test-form").onsubmit=runAPITest;$("#api-test-stop").onclick=stopAPITest;$("#api-test-model").onchange=syncAPITestModelLimits;
  $$("[data-close]").forEach(b=>b.onclick=()=>closeDialog(b.dataset.close));
  $$("[data-copy]").forEach(b=>b.onclick=async()=>{const el=$("#"+b.dataset.copy);await navigator.clipboard.writeText(el.textContent);toast("已复制","good");});
  document.addEventListener("click",e=>{const d=$("#top-more");if(d?.open&&!d.contains(e.target))closeTopMore();});
  window.addEventListener("message",e=>{if(e.origin===location.origin&&e.data?.type==="verdent-auth"&&e.data.status==="success")toast("授权回调已完成，正在保存账号…","good");});
}
(async function init(){
  initTheme();bind();
  try{const s=await api("/auth/session");$("#admin-user").textContent=s.username||"admin";}catch{return;}
  $("#base-url").textContent=location.origin+"/v1";
  await loadOverview();
})();
