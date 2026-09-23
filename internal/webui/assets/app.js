const $=(s,r=document)=>r.querySelector(s);
const $$=(s,r=document)=>[...r.querySelectorAll(s)];
const PAGES={overview:"概览",accounts:"账号",models:"模型",keys:"密钥",api:"API"};
let currentPage="overview";
let accountCache=[];
let keysCache=[];

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
  $(".page").forEach(x=>x.hidden=x.id!=="page-"+page);
  $("#page-title").textContent=PAGES[page]||page;
  closeTopMore();
  const drawer=$("#mobile-drawer");if(drawer?.open)drawer.close();
  loadPage(page).catch(e=>toast(e.message,"bad"));
}
async function loadPage(page){
  if(page==="overview")return loadOverview();
  if(page==="accounts")return loadAccounts();
  if(page==="models")return loadModels();
  if(page==="keys")return loadKeys();
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
  $("#accounts-table").innerHTML='<div class="table-wrap"><table class="data-table responsive-table"><thead><tr><th>账号</th><th>状态</th><th>Token</th><th>Device / Team</th><th>固定代理</th><th>操作</th></tr></thead><tbody>'+accountCache.map(a=>{
    const expiry=a.token_expires?fmtTime(a.token_expires):"未知";
    const proxy=a.proxy_url||"直连";
    return '<tr>'+
      '<td data-label="账号"><div class="cell-main">'+esc(a.label||a.id)+'</div><div class="cell-sub mono">'+esc(a.id)+'</div></td>'+
      '<td data-label="状态">'+stateBadge(a.state)+(a.last_error?'<div class="cell-sub wrap-anywhere">'+esc(a.last_error.slice(0,100))+'</div>':"")+'</td>'+
      '<td data-label="Token"><div class="cell-main">'+esc(a.token_uid||"—")+'</div><div class="cell-sub">过期 '+esc(expiry)+'</div></td>'+
      '<td data-label="设备"><div class="mono wrap-anywhere">'+esc(a.device_id||"—")+'</div><div class="cell-sub">Team '+esc(a.team_id||"0")+'</div></td>'+
      '<td data-label="固定代理"><div class="mono wrap-anywhere">'+esc(proxy)+'</div></td>'+
      '<td data-label="操作"><div class="table-actions">'+
      '<button class="table-action" data-test="'+esc(a.id)+'">'+icon("check","mini-icon")+'<span>测试</span></button>'+
      '<button class="table-action" data-proxy="'+esc(a.id)+'">'+icon("link","mini-icon")+'<span>代理</span></button>'+
      '<button class="table-action" data-toggle="'+esc(a.id)+'">'+icon("power","mini-icon")+'<span>'+(a.state==="disabled"?"启用":"停用")+'</span></button>'+
      '<button class="table-action danger" data-delete="'+esc(a.id)+'">'+icon("trash","mini-icon")+'<span>删除</span></button>'+
      '</div></td></tr>';
  }).join("")+"</tbody></table></div>";
  $$("[data-test]").forEach(b=>b.onclick=()=>testAccount(b.dataset.test,b));
  $$("[data-proxy]").forEach(b=>b.onclick=()=>openProxy(b.dataset.proxy));
  $$("[data-toggle]").forEach(b=>b.onclick=()=>toggleAccount(b.dataset.toggle));
  $$("[data-delete]").forEach(b=>b.onclick=()=>deleteAccount(b.dataset.delete));
}

async function testAccount(id,button){
  button.disabled=true;
  try{const r=await api("/accounts/"+encodeURIComponent(id)+"/test",{method:"POST"});toast("连接成功，发现 "+r.models+" 个模型","good");}
  catch(e){toast(e.message,"bad");}
  finally{button.disabled=false;}
}
async function toggleAccount(id){
  const a=accountCache.find(x=>x.id===id);if(!a)return;
  await api("/accounts/"+encodeURIComponent(id)+"/state",{method:"POST",body:{disabled:a.state!=="disabled"}});
  toast(a.state==="disabled"?"账号已启用":"账号已停用","good");await loadAccounts();await loadOverview();
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
async function loadAPI(){$("#base-url").textContent=location.origin+"/v1";}

async function loadKeys(){
  const d=await api("/keys");
  keysCache=d.keys||[];
  $("#keys-meta").textContent=keysCache.length?keysCache.length+" 个密钥 · "+(d.store_file||""):"还没有密钥。";
  if(!keysCache.length){$("#keys-table").innerHTML='<div class="empty">还没有密钥。点击右上角「创建密钥」。</div>';return;}
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

async function logout(){try{await api("/auth/logout",{method:"POST"});}catch{}location.replace("/login");}
function bind(){
  $("[data-page]").forEach(b=>b.onclick=()=>setPage(b.dataset.page));
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
  $("[data-close]").forEach(b=>b.onclick=()=>closeDialog(b.dataset.close));
  $("[data-copy]").forEach(b=>b.onclick=async()=>{const el=$("#"+b.dataset.copy);await navigator.clipboard.writeText(el.textContent);toast("已复制","good");});
  document.addEventListener("click",e=>{const d=$("#top-more");if(d?.open&&!d.contains(e.target))closeTopMore();});
  window.addEventListener("message",e=>{if(e.origin===location.origin&&e.data?.type==="verdent-auth"&&e.data.status==="success")toast("授权回调已完成，正在保存账号…","good");});
}
(async function init(){
  initTheme();bind();
  try{const s=await api("/auth/session");$("#admin-user").textContent=s.username||"admin";}catch{return;}
  $("#base-url").textContent=location.origin+"/v1";
  await loadOverview();
})();
