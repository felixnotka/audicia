// Analytics beacon. The gateway answers /_b with 204 and writes the request to
// the ingress access log, which is what the platform's traffic pipeline reads –
// so a hit needs no endpoint, no service and no request to a third party.
//
// Why a beacon at all: automated clients do not execute JavaScript, so a hit is
// the only positive evidence that a browser ran. It is not evidence of a person,
// because rendering crawlers execute scripts too. Load and first interaction are
// therefore reported as separate events and the consumer decides what to count.
//
// Nothing is stored on the device – no cookie, no localStorage, no identifier –
// so the beacon reports a page view and never a visitor across sites.

// Three things, because one of them alone is never enough:
//
//   p  the route pattern - /blog/:slug, so a blog is one line and not one per
//      article. Only the server knows it, which is why this lives in the template
//      and not in the gateway. Fresh leaves it null on an error page and the
//      caller falls back to the pathname.
//   u  the resolved path, so the grouped line can be opened. Reporting the
//      pattern alone said a visitor looked at ":id" three times and could not say
//      which - the most interesting figure on a report, rendered useless.
//   t  document.title, which is what the page calls itself. "Winterkurs 2026/27"
//      is what a site owner recognises; /termine/6 is not. Public page content,
//      nothing personal.
export function beaconScript(route: string): string {
  const encoded = JSON.stringify(route).replaceAll("<", String.raw`\u003c`);

  return `(function(){
var R=${encoded},sent={};
function send(i){
if(sent[i])return;sent[i]=1;
var q='/_b?p='+encodeURIComponent(R)+'&i='+i;
if(!i){
q+='&u='+encodeURIComponent(location.pathname);
var T=(document.title||'').slice(0,120);
if(T)q+='&t='+encodeURIComponent(T);
try{var f=document.referrer;if(f){var h=new URL(f).hostname;
if(h&&h!==location.hostname)q+='&r='+encodeURIComponent(h);}}catch(e){}}
if(window.fetch)fetch(q,{method:'GET',keepalive:true,cache:'no-store'}).catch(function(){});
else new Image().src=q;}
function load(){send(0);}
if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',load);else load();
var E=['pointerdown','scroll','keydown','touchstart'];
function hit(){for(var i=0;i<E.length;i++)removeEventListener(E[i],hit,true);send(1);}
for(var i=0;i<E.length;i++)addEventListener(E[i],hit,{capture:true,passive:true});
})();`;
}
