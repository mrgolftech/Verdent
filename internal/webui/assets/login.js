const $=(s)=>document.querySelector(s);
$("#login-form").addEventListener("submit",async e=>{
  e.preventDefault();$("#login-error").textContent="";
  const button=e.currentTarget.querySelector("button");button.disabled=true;
  try{
    const res=await fetch("/api/auth/login",{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:JSON.stringify({username:$("#username").value.trim(),password:$("#password").value})});
    let data={};try{data=await res.json();}catch{}
    if(!res.ok)throw new Error(data.error||"登录失败");
    location.replace("/");
  }catch(err){$("#login-error").textContent=err.message;}
  finally{button.disabled=false;}
});
