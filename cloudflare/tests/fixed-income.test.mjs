import test from 'node:test';
import assert from 'node:assert/strict';
import {
  xirr, xnpv, theoreticalPriceFromYield, maxPurchasePriceForNetYield, analyzeFixedIncome,
  parseCashFlowText, spreadMetrics, al30Example, lqc1dExample
} from '../public/fixed-income-core.mjs';

test('XIRR simple bullet 10% a un año', () => {
  const flows = [
    { date: '2026-01-01', amount: -100 },
    { date: '2027-01-01', amount: 110 },
  ];
  const r = xirr(flows, 'ACT/365');
  assert.ok(r !== null);
  assert.ok(Math.abs(r - 0.10) < 1e-6, `xirr=${r}`);
});

test('precio por TIR y XIRR son consistentes con amortización parcial', () => {
  const input = {
    paymentCurrency: 'USD', quoteCurrency: 'USD', quoteScale: 100, flowBase: 100,
    priceType: 'dirty', settlementDate: '2026-01-01', dayCount: 'ACT/365',
    flows: [
      { date: '2026-07-01', interest: 5, amortization: 50 },
      { date: '2027-01-01', interest: 2.5, amortization: 50 },
    ]
  };
  const price = theoreticalPriceFromYield(input, 10);
  assert.ok(price > 90 && price < 105);
  const r = xirr([
    {date:'2026-01-01', amount:-price},
    {date:'2026-07-01', amount:55},
    {date:'2027-01-01', amount:52.5},
  ], 'ACT/365');
  assert.ok(Math.abs(r - 0.10) < 1e-6, `price=${price}, xirr=${r}`);
});

test('parsea calendario CSV/semicolon', () => {
  const p = parseCashFlowText('fecha;interes;amortizacion\n2027-01-09;0,24;8\n2027-07-09;0.21;8');
  assert.equal(p.errors.length, 0);
  assert.equal(p.rows.length, 2);
  assert.equal(p.rows[0].amortization, 8);
});

test('spread calcula costo punta a punta', () => {
  const s = spreadMetrics(102, 100);
  assert.ok(Math.abs(s.pctAsk - 1.9607843137) < 1e-6);
});

test('AL30 preset conserva residual futuro 64 sin inventar capital como ganancia', () => {
  const e = al30Example();
  const amort = e.flows.reduce((a,f)=>a+f.amortization,0);
  assert.equal(amort, 64);
  assert.equal(e.historicalExample, true);
});

test('LQC1D preset no inventa calendario exacto de amortización', () => {
  const e = lqc1dExample();
  assert.equal(e.flows.length, 0);
  assert.match(e.amortizationType, /33%/);
});

test('evaluación no habilita oportunidad atractiva sin calendario', () => {
  const a = analyzeFixedIncome({
    ticker:'TEST', paymentCurrency:'USD', quoteCurrency:'USD', quoteScale:100, flowBase:100,
    askPrice:100, bidPrice:99.5, quoteTimestamp:new Date().toISOString(), settlementDate:new Date().toISOString().slice(0,10),
    targetYieldPct:9, maxSpreadPct:2, creditRisk:'bajo', duration:3, flows:[]
  });
  assert.equal(a.decisionKind, 'insufficient');
});

test('XNPV del XIRR resuelto queda cerca de cero', () => {
  const flows=[{date:'2026-01-01',amount:-95},{date:'2026-07-01',amount:4},{date:'2027-01-01',amount:104}];
  const r=xirr(flows,'ACT/365');
  assert.ok(r!==null);
  assert.ok(Math.abs(xnpv(r,flows,'ACT/365'))<1e-7);
});

test('precio máximo neto baja cuando hay comisión de compra', () => {
  const input = { paymentCurrency:'USD', quoteCurrency:'USD', quoteScale:100, flowBase:100, priceType:'dirty', settlementDate:'2026-01-01', dayCount:'ACT/365', buyFeePct:1, flows:[{date:'2027-01-01',interest:10,amortization:100}] };
  const gross = theoreticalPriceFromYield(input, 10);
  const net = maxPurchasePriceForNetYield(input, 10);
  assert.ok(net < gross);
  assert.ok(Math.abs(net - gross/1.01) < 1e-8);
});

test('detecta puntas invertidas y no calcula spread engañoso', () => {
  const s = spreadMetrics(844.5, 860);
  assert.equal(s.inverted, true);
  assert.equal(s.pctAsk, null);
  const a = analyzeFixedIncome({
    ticker:'TEST', paymentCurrency:'USD', quoteCurrency:'USD', quoteScale:100, flowBase:100,
    askPrice:844.5, bidPrice:860, quoteTimestamp:new Date().toISOString(), settlementDate:new Date().toISOString().slice(0,10),
    targetYieldPct:9, maxSpreadPct:2, creditRisk:'bajo', duration:3,
    flows:[{date:'2030-01-01',interest:10,amortization:100}]
  });
  assert.equal(a.decisionKind, 'insufficient');
  assert.ok(a.limitations.some(x=>x.includes('puntas')));
});
