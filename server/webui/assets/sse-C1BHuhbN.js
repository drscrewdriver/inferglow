function i(d){const c=[];let t=d.replace(/\r\n/g,`
`);for(;;){const f=t.indexOf(`

`);if(f===-1)break;const o=t.slice(0,f);t=t.slice(f+2);const n={event:"",data:""};for(const r of o.split(`
`))r.startsWith("event:")?n.event=r.slice(6).trim():r.startsWith("data:")&&(n.data=r.slice(5).trim());c.push(n)}return{frames:c,rest:t}}async function p(d,c,e){if(!d.body)return;const t=d.body.getReader(),f=new TextDecoder("utf-8");let o="";const n=()=>{t.cancel()};e==null||e.addEventListener("abort",n,{once:!0});try{for(;;){const{done:s,value:m}=await t.read();if(s)break;o+=f.decode(m,{stream:!0});const{frames:u,rest:b}=i(o);o=b;for(const v of u)c(v);if(e!=null&&e.aborted)return}const{frames:r,rest:a}=i(o);if(a.trim()!==""){const s={event:"",data:a.trim()};r.push(s)}for(const s of r)c(s)}finally{e==null||e.removeEventListener("abort",n)}}export{p as parseSSE,i as splitFrames};
