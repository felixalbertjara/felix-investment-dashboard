import { analyzeFixedIncome, parseCashFlowText, al30Example, lqc1dExample, num } from './fixed-income-core.mjs';

const $fi = s => document.querySelector(s);
let lastInput = null;
let lastAnalysis = null;
let savedItems = [];
let timer = null;

function esc(s){return String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));}
function fmtN(v,d=2){return Number.isFinite(Number(v))?Number(v).toLocaleString('es-AR',{minimumFractionDigits:d,maximumFractionDigits:d}):'-';}
function fmtPct(v){return Number.isFinite(Number(v))?`${fmtN(v,2)}%`:'-';}
function fmtMoney(v,c='USD'){if(!Number.isFinite(Number(v)))return'-';try{return new Intl.NumberFormat('es-AR',{style:'currency',currency:c||'USD',maximumFractionDigits:2}).format(Number(v));}catch{return `${c||''} ${fmtN(v,2)}`;}}
function toastFi(msg){const t=document.querySelector('#toast');if(!t)return alert(msg);t.textContent=msg;t.style.display='block';setTimeout(()=>t.style.display='none',4200);}
async function apiFi(path,opts={}){const r=await fetch(path,{...opts,headers:{'content-type':'application/json',...(opts.headers||{})}});const j=await r.json().catch(()=>({}));if(!r.ok)throw new Error(j.error||`HTTP ${r.status}`);return j;}

function injectStyles(){
  if(document.querySelector('#fiStyles'))return;
  const st=document.createElement('style');st.id='fiStyles';st.textContent=`
  #fixedIncome{scroll-margin-top:95px}.fi-panel{border:1px solid var(--line);background:rgba(13,20,36,.82);border-radius:20px;padding:16px;margin-bottom:16px}.fi-grid{display:grid;grid-template-columns:repeat(4,minmax(180px,1fr));gap:10px}.fi-grid label,.fi-flowbox label{display:flex;flex-direction:column;gap:6px;color:#b9c6e4;font-size:11px;text-transform:uppercase;letter-spacing:.04em}.fi-grid input,.fi-grid select,.fi-grid textarea{width:100%;min-width:0}.fi-grid .wide{grid-column:span 2}.fi-grid .full{grid-column:1/-1}.fi-actions{display:flex;gap:9px;flex-wrap:wrap;margin-top:12px}.fi-actions button{border-radius:999px}.fi-details{margin-top:12px;border-top:1px solid var(--line);padding-top:10px}.fi-details summary{cursor:pointer;font-weight:900;color:#dbeafe}.fi-result-grid{display:grid;grid-template-columns:repeat(4,minmax(180px,1fr));gap:10px;margin-top:12px}.fi-result{border:1px solid var(--line);background:#081020;border-radius:14px;padding:12px}.fi-result .k{font-size:10px;color:var(--muted);text-transform:uppercase}.fi-result .v{font-size:18px;font-weight:950;margin-top:6px}.fi-decision{font-size:20px;font-weight:950;margin-bottom:8px}.fi-two{display:grid;grid-template-columns:1fr 1fr;gap:12px}.fi-list{margin:5px 0 0;padding-left:18px}.fi-list li{margin:5px 0}.fi-good{color:#86efac}.fi-warn{color:#fde68a}.fi-bad{color:#fca5a5}.fi-muted{color:var(--muted)}.fi-table{width:100%;min-width:900px}.fi-table th{position:static}.fi-chart{width:100%;min-height:180px;background:#081020;border:1px solid var(--line);border-radius:14px;padding:10px;overflow:hidden}.fi-flowtext{width:100%;min-height:150px;resize:vertical}.fi-pill{display:inline-flex;border:1px solid var(--line);border-radius:999px;padding:4px 8px;font-size:11px;font-weight:900}.fi-manual{color:#fde68a;border-color:rgba(245,158,11,.4);background:rgba(245,158,11,.1)}.fi-toolbar{display:flex;gap:8px;align-items:center;flex-wrap:wrap}.fi-toolbar select{min-width:180px}.fi-small{font-size:11px;color:var(--muted);line-height:1.45}.fi-hidden{display:none!important}@media(max-width:1000px){.fi-grid,.fi-result-grid{grid-template-columns:repeat(2,minmax(160px,1fr))}.fi-two{grid-template-columns:1fr}}@media(max-width:560px){.fi-grid,.fi-result-grid{grid-template-columns:1fr}.fi-grid .wide,.fi-grid .full{grid-column:auto}.fi-panel{padding:12px}}
  `;document.head.appendChild(st);
}

function formToInput(){
  const f=$fi('#fiForm');const fd=new FormData(f);const o=Object.fromEntries(fd.entries());
  ['askPrice','bidPrice','investmentAmount','couponAnnualPct','frequency','residualPct','quoteScale','flowBase','accruedInterest','fxRateQuotePerPayment','exitFxRateQuotePerPayment','balanzYtmPct','tnaPct','parityPct','technicalValue','duration','modifiedDuration','pendingPayments','volume','low52','high52','buyFeePct','sellFeePct','targetSellPrice','targetYieldPct','maxSpreadPct','maxDurationYears','netDebtEbitda','interestCoverage','alertPriceMax','alertYieldMinPct','alertSpreadMaxPct'].forEach(k=>o[k]=o[k]===''?'':num(o[k],0));
  const parsed=parseCashFlowText(o.cashFlowText||'');o.flows=parsed.rows;o.cashFlowErrors=parsed.errors;delete o.cashFlowText;
  o.historicalExample=$fi('#fiHistoricalExample').value==='1';
  return o;
}

function cashFlowsToText(flows){return (flows||[]).map(f=>`${f.date};${f.interest};${f.amortization}`).join('\n');}
function setForm(data){
  const f=$fi('#fiForm');f.reset();
  for(const [k,v] of Object.entries(data||{})){
    const el=f.elements.namedItem(k);if(!el||k==='flows')continue;
    el.value=v??'';
  }
  $fi('#fiCashFlowText').value=cashFlowsToText(data?.flows||[]);
  $fi('#fiHistoricalExample').value=data?.historicalExample?'1':'0';
  runAnalysis();
}

function decisionClass(kind){return kind==='attractive'?'fi-good':kind==='risk'?'fi-bad':kind==='wait'?'fi-warn':'fi-muted';}
function renderAnalysis(input,a){
  const box=$fi('#fiAnalysis');if(!box)return;
  const pc=input.paymentCurrency||'USD', qc=input.quoteCurrency||pc;
  const errors=input.cashFlowErrors||[];
  const alertRows=(a.manualAlerts||[]).map(x=>`<div class="fi-result"><div class="k">Alerta ${esc(x.type)}</div><div class="v ${x.hit?'fi-good':'fi-muted'}">${x.hit?'CONDICIÓN CUMPLIDA':'En espera'}</div><div class="fi-small">${esc(x.text)}<br><span class="fi-pill fi-manual">${esc(x.verification)}</span></div></div>`).join('');
  box.innerHTML=`
    <div class="fi-panel">
      <div class="fi-decision ${decisionClass(a.decisionKind)}">${esc(a.decision)}</div>
      <div class="fi-small">Confianza: <b>${esc(a.confidence)}</b>. La señal evalúa rendimiento, riesgo crediticio, spread, duration, precio objetivo y calidad de datos; nunca se vuelve verde sólo por una TIR alta.</div>
      <div class="fi-result-grid">
        <div class="fi-result"><div class="k">TIR calculada bruta</div><div class="v">${fmtPct(a.grossYtmPct)}</div><div class="fi-small">Con flujos y fechas reales cargadas.</div></div>
        <div class="fi-result"><div class="k">TIR neta estimada</div><div class="v">${fmtPct(a.netYtmPct)}</div><div class="fi-small">Incluye comisión de compra.</div></div>
        <div class="fi-result"><div class="k">Precio máximo por objetivo</div><div class="v">${a.idealPrice==null?'-':fmtMoney(a.idealPrice,qc)}</div><div class="fi-small">Para TIR objetivo ${fmtPct(input.targetYieldPct)}; no equivale automáticamente a valor justo.</div></div>
        <div class="fi-result"><div class="k">Spread punta a punta</div><div class="v">${fmtPct(a.spread?.pctAsk)}</div><div class="fi-small">Costo inmediato aproximado entre vendedor y comprador.</div></div>
        <div class="fi-result"><div class="k">VN estimado comprado</div><div class="v">${a.nominalPurchased==null?'-':fmtN(a.nominalPurchased,2)}</div><div class="fi-small">Con el monto ingresado y comisión de compra.</div></div>
        <div class="fi-result"><div class="k">Intereses pendientes</div><div class="v">${fmtMoney(a.totalInterest,pc)}</div></div>
        <div class="fi-result"><div class="k">Capital a recuperar</div><div class="v">${fmtMoney(a.capitalRecovered,pc)}</div><div class="fi-small">No se cuenta como ganancia.</div></div>
        <div class="fi-result"><div class="k">Ganancia neta a vencimiento</div><div class="v ${Number(a.netGainMaturity)>=0?'fi-good':'fi-bad'}">${fmtMoney(a.netGainMaturity,pc)}</div><div class="fi-small">Retorno total ${fmtPct(a.totalReturnPct)} sin reinvertir cupones.</div></div>
      </div>
    </div>
    <div class="fi-two">
      <div class="fi-panel"><b>✅ Factores a favor</b><ul class="fi-list fi-good">${(a.reasons||[]).length?(a.reasons||[]).map(x=>`<li>${esc(x)}</li>`).join(''):'<li>Sin factores suficientes para destacar.</li>'}</ul></div>
      <div class="fi-panel"><b>⚠ Riesgos / limitaciones</b><ul class="fi-list fi-warn">${[...(a.warnings||[]),...(a.limitations||[]),...errors].length?[...(a.warnings||[]),...(a.limitations||[]),...errors].map(x=>`<li>${esc(x)}</li>`).join(''):'<li>Sin advertencias relevantes con los datos ingresados.</li>'}</ul></div>
    </div>
    <div class="fi-panel"><h3 style="margin-top:0">Venta antes del vencimiento</h3>${renderSale(input,a)}</div>
    <div class="fi-two">
      <div class="fi-panel"><h3 style="margin-top:0">Sensibilidad si cambia la TIR exigida</h3>${renderSensitivity(a,input)}</div>
      <div class="fi-panel"><h3 style="margin-top:0">Pagos futuros</h3>${renderFlowChart(input)}</div>
    </div>
    ${alertRows?`<div class="fi-panel"><h3 style="margin-top:0">Alertas configuradas</h3><div class="fi-result-grid">${alertRows}</div><div class="fi-small" style="margin-top:10px">Como las cotizaciones de Bonos & ONs se cargan manualmente, estas condiciones <b>no son monitoreo en tiempo real</b> y no disparan Telegram automático. Quedan guardadas para comparar contra el próximo dato que cargues.</div></div>`:''}
  `;
}

function renderSale(input,a){const s=a.sale;if(!s?.available)return `<div class="fi-muted">${esc(s?.reason||'Completá precio objetivo y fecha de venta.')}</div>`;return `<div class="fi-result-grid"><div class="fi-result"><div class="k">Rentabilidad escenario</div><div class="v">${fmtPct(s.returnPct)}</div></div><div class="fi-result"><div class="k">Rentabilidad anualizada</div><div class="v">${fmtPct(s.annualizedPct)}</div></div><div class="fi-result"><div class="k">Break-even de venta</div><div class="v">${fmtMoney(s.breakEvenSalePrice,input.quoteCurrency||'USD')}</div><div class="fi-small">Precio aprox. para recuperar costos considerando pagos previos.</div></div><div class="fi-result"><div class="k">Cobros antes de vender</div><div class="v">${fmtMoney(s.interimCashPerFlowBase,input.paymentCurrency||'USD')}</div><div class="fi-small">Por cada base de flujo ingresada.</div></div></div>`;}
function renderSensitivity(a,input){if(!(a.sensitivity||[]).length)return '<div class="fi-muted">Cargá calendario de flujos y TIR de mercado/calculada para simular ±1/±2 puntos.</div>';return `<div style="overflow:auto"><table class="fi-table"><thead><tr><th>Cambio TIR</th><th>TIR escenario</th><th>Precio teórico</th><th>Vs compra</th></tr></thead><tbody>${a.sensitivity.map(x=>`<tr><td>${x.deltaPctPoints>0?'+':''}${x.deltaPctPoints.toFixed(0)} p.p.</td><td>${fmtPct(x.yieldPct)}</td><td>${fmtMoney(x.price,input.quoteCurrency||'USD')}</td><td class="${Number(x.pnlPct)>=0?'fi-good':'fi-bad'}">${fmtPct(x.pnlPct)}</td></tr>`).join('')}</tbody></table></div>`;}
function renderFlowChart(input){const rows=(input.flows||[]);if(!rows.length)return '<div class="fi-muted">Pegá/importá el calendario para ver el gráfico.</div>';const vals=rows.map(x=>num(x.interest)+num(x.amortization)),max=Math.max(...vals,1),w=720,h=180,pad=26,bw=Math.max(6,(w-pad*2)/rows.length*.65);const bars=rows.map((r,i)=>{const x=pad+(i+.5)*(w-pad*2)/rows.length-bw/2;const bh=vals[i]/max*(h-55);const y=h-30-bh;return `<rect x="${x.toFixed(1)}" y="${y.toFixed(1)}" width="${bw.toFixed(1)}" height="${bh.toFixed(1)}" rx="3" fill="currentColor" opacity=".75"><title>${esc(r.date)} · ${fmtN(vals[i],2)}</title></rect><text x="${(x+bw/2).toFixed(1)}" y="${h-12}" text-anchor="middle" font-size="8" fill="#8ea0c4">${esc(r.date.slice(2,7))}</text>`}).join('');return `<div class="fi-chart"><svg viewBox="0 0 ${w} ${h}" width="100%" height="180" style="color:#38bdf8"><line x1="${pad}" y1="${h-30}" x2="${w-pad}" y2="${h-30}" stroke="#22304d"/>${bars}</svg><div class="fi-small">Interés + amortización por fecha, en la moneda de pago y por la base de flujo indicada.</div></div>`;}

function runAnalysis(){
  try{const input=formToInput();const a=analyzeFixedIncome(input);lastInput=input;lastAnalysis=a;renderAnalysis(input,a);return a;}catch(e){console.error(e);toastFi(`Bonos & ONs: ${e.message}`);return null;}
}

async function saveCurrent(){
  const a=runAnalysis();if(!a||!lastInput?.ticker)return toastFi('Ingresá al menos el ticker y los datos del instrumento.');
  const record={ticker:String(lastInput.ticker).trim().toUpperCase(),issuer:lastInput.issuer||'',payment_currency:lastInput.paymentCurrency||'',quote_currency:lastInput.quoteCurrency||'',updated_at:new Date().toISOString(),input:lastInput,summary:{decision:a.decision,decisionKind:a.decisionKind,confidence:a.confidence,netYtmPct:a.netYtmPct,grossYtmPct:a.grossYtmPct,spreadPct:a.spread?.pctAsk,duration:num(lastInput.modifiedDuration||lastInput.duration,0),idealPrice:a.idealPrice,quoteTimestamp:lastInput.quoteTimestamp||'',manualAlerts:a.manualAlerts||[]}};
  try{await apiFi('/api/fixed-income/save',{method:'POST',body:JSON.stringify(record)});toastFi(`${record.ticker} guardado en Bonos & ONs.`);await loadSaved();}catch(e){toastFi(`No pude guardar: ${e.message}`);}
}

async function loadSaved(){try{const d=await apiFi('/api/fixed-income');savedItems=Array.isArray(d.instruments)?d.instruments:[];renderSaved();}catch(e){console.error(e);}}
async function deleteSaved(t){if(!confirm(`¿Eliminar ${t} del comparador?`))return;try{await apiFi('/api/fixed-income/delete',{method:'POST',body:JSON.stringify({ticker:t})});await loadSaved();toastFi(`${t} eliminado.`);}catch(e){toastFi(e.message)}}
function loadSavedIntoForm(t){const r=savedItems.find(x=>String(x.ticker).toUpperCase()===String(t).toUpperCase());if(r?.input)setForm(r.input);location.hash='#fixedIncome';}
function evalRank(k){return k==='attractive'?0:k==='wait'?1:k==='risk'?2:3;}
function renderSaved(){
  const tb=$fi('#fiSavedRows');if(!tb)return;const currency=$fi('#fiCompareCurrency')?.value||'';const sort=$fi('#fiCompareSort')?.value||'yield';
  let rows=savedItems.filter(x=>!currency||x.payment_currency===currency);
  rows=[...rows].sort((a,b)=>{const A=a.summary||{},B=b.summary||{};if(sort==='spread')return num(A.spreadPct,999)-num(B.spreadPct,999);if(sort==='duration')return num(A.duration,999)-num(B.duration,999);if(sort==='decision')return evalRank(A.decisionKind)-evalRank(B.decisionKind);return num(B.netYtmPct,-999)-num(A.netYtmPct,-999)});
  if(!rows.length){tb.innerHTML='<tr><td colspan="10" class="empty">Todavía no guardaste instrumentos en esta moneda.</td></tr>';return;}
  tb.innerHTML=rows.map(r=>{const s=r.summary||{},i=r.input||{};return `<tr><td><div class="ticker">${esc(r.ticker)}</div><div class="muted">${esc(r.issuer)}</div></td><td>${esc(r.payment_currency||'-')}</td><td>${esc(i.maturityDate||'-')}</td><td>${fmtMoney(num(i.askPrice,NaN),r.quote_currency||'USD')}</td><td>${fmtPct(s.grossYtmPct)}</td><td>${fmtPct(s.netYtmPct)}</td><td>${fmtN(s.duration,2)}</td><td>${fmtPct(s.spreadPct)}</td><td class="${decisionClass(s.decisionKind)}"><b>${esc(s.decision||'-')}</b><div class="fi-small">${esc(s.confidence||'')}</div></td><td><button onclick="window.FixedIncomeUI.loadSaved('${esc(r.ticker)}')">Abrir</button> <button class="danger" onclick="window.FixedIncomeUI.deleteSaved('${esc(r.ticker)}')">Eliminar</button></td></tr>`}).join('');
}

function preset(kind){setForm(kind==='al30'?al30Example():lqc1dExample());toastFi('Ejemplo histórico cargado. No se usa como recomendación actual.');}
function resetForm(){const f=$fi('#fiForm');f.reset();$fi('#fiHistoricalExample').value='0';$fi('#fiCashFlowText').value='';lastInput=null;lastAnalysis=null;$fi('#fiAnalysis').innerHTML='<div class="fi-panel fi-muted">Completá los datos y RADAR analizará la inversión sin inventar los datos faltantes.</div>';}

function bind(){
  const f=$fi('#fiForm');if(!f)return;
  f.addEventListener('submit',e=>{e.preventDefault();runAnalysis()});
  f.addEventListener('input',()=>{clearTimeout(timer);timer=setTimeout(runAnalysis,350)});
  $fi('#fiAnalyzeBtn')?.addEventListener('click',runAnalysis);
  $fi('#fiSaveBtn')?.addEventListener('click',saveCurrent);
  $fi('#fiResetBtn')?.addEventListener('click',resetForm);
  $fi('#fiPresetAL30')?.addEventListener('click',()=>preset('al30'));
  $fi('#fiPresetLQC1D')?.addEventListener('click',()=>preset('lqc1d'));
  $fi('#fiCompareCurrency')?.addEventListener('change',renderSaved);$fi('#fiCompareSort')?.addEventListener('change',renderSaved);
  $fi('#fiCsvFile')?.addEventListener('change',async e=>{const file=e.target.files?.[0];if(!file)return;const txt=await file.text();$fi('#fiCashFlowText').value=txt;runAnalysis();e.target.value='';});
  runAnalysis();loadSaved();
}

injectStyles();
window.FixedIncomeUI={runAnalysis,saveCurrent,loadSaved:loadSavedIntoForm,deleteSaved,preset,resetForm};
if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',bind);else bind();
