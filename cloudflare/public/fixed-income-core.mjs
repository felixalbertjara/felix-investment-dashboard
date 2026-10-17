const EPS = 1e-10;

export function num(v, fallback = 0) {
  if (typeof v === 'number') return Number.isFinite(v) ? v : fallback;
  if (v == null || v === '') return fallback;
  let s = String(v).trim().replace(/\s/g, '');
  if (!s) return fallback;
  // Admite 1.234,56 y 1234.56 sin intentar adivinar cifras enteras ambiguas.
  if (s.includes(',') && s.includes('.')) {
    if (s.lastIndexOf(',') > s.lastIndexOf('.')) s = s.replace(/\./g, '').replace(',', '.');
    else s = s.replace(/,/g, '');
  } else if (s.includes(',')) {
    s = s.replace(',', '.');
  }
  const n = Number(s);
  return Number.isFinite(n) ? n : fallback;
}

export function isoDate(v) {
  if (!v) return '';
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return '';
  return d.toISOString().slice(0, 10);
}

function asDate(v) {
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? null : d;
}

export function yearFraction(start, end, convention = 'ACT/365') {
  const a = asDate(start), b = asDate(end);
  if (!a || !b) return NaN;
  const msDays = (b - a) / 86400000;
  const c = String(convention || 'ACT/365').toUpperCase();
  if (c === 'ACT/360') return msDays / 360;
  if (c === '30/360') {
    let d1 = a.getUTCDate(), d2 = b.getUTCDate();
    const m1 = a.getUTCMonth() + 1, m2 = b.getUTCMonth() + 1;
    const y1 = a.getUTCFullYear(), y2 = b.getUTCFullYear();
    if (d1 === 31) d1 = 30;
    if (d2 === 31 && d1 >= 30) d2 = 30;
    return ((y2 - y1) * 360 + (m2 - m1) * 30 + (d2 - d1)) / 360;
  }
  return msDays / 365;
}

export function xnpv(rate, flows, convention = 'ACT/365') {
  if (!(rate > -1) || !Array.isArray(flows) || !flows.length) return NaN;
  const valid = flows
    .map(f => ({ date: asDate(f.date), amount: num(f.amount, NaN) }))
    .filter(f => f.date && Number.isFinite(f.amount))
    .sort((a, b) => a.date - b.date);
  if (!valid.length) return NaN;
  const t0 = valid[0].date;
  let out = 0;
  for (const f of valid) {
    const yf = yearFraction(t0, f.date, convention);
    const den = Math.pow(1 + rate, yf);
    if (!Number.isFinite(den) || Math.abs(den) < EPS) return NaN;
    out += f.amount / den;
  }
  return out;
}

export function xirr(flows, convention = 'ACT/365') {
  const vals = (flows || []).map(f => num(f.amount, NaN)).filter(Number.isFinite);
  if (!vals.some(v => v < 0) || !vals.some(v => v > 0)) return null;
  const candidates = [-0.9999, -0.9, -0.75, -0.5, -0.25, -0.1, 0, 0.02, 0.05, 0.1, 0.2, 0.4, 0.75, 1, 2, 5, 10, 25, 50, 100];
  let lo = null, hi = null, flo = null;
  for (let i = 0; i < candidates.length - 1; i++) {
    const a = candidates[i], b = candidates[i + 1];
    const fa = xnpv(a, flows, convention), fb = xnpv(b, flows, convention);
    if (!Number.isFinite(fa) || !Number.isFinite(fb)) continue;
    if (Math.abs(fa) < 1e-8) return a;
    if (fa * fb <= 0) { lo = a; hi = b; flo = fa; break; }
  }
  if (lo == null) return null;
  for (let i = 0; i < 160; i++) {
    const mid = (lo + hi) / 2;
    const fm = xnpv(mid, flows, convention);
    if (!Number.isFinite(fm)) return null;
    if (Math.abs(fm) < 1e-10 || Math.abs(hi - lo) < 1e-10) return mid;
    if (flo * fm <= 0) hi = mid;
    else { lo = mid; flo = fm; }
  }
  return (lo + hi) / 2;
}

export function parseCashFlowText(text) {
  const rows = [];
  const errors = [];
  String(text || '').split(/\r?\n/).forEach((raw, idx) => {
    const line = raw.trim();
    if (!line || /^fecha[;,\t]/i.test(line)) return;
    const sep = line.includes(';') ? ';' : line.includes('\t') ? '\t' : ',';
    const p = line.split(sep).map(x => x.trim());
    if (p.length < 2) { errors.push(`Línea ${idx + 1}: faltan columnas.`); return; }
    const date = isoDate(p[0]);
    const interest = num(p[1], NaN);
    const amortization = num(p[2] || 0, NaN);
    if (!date || !Number.isFinite(interest) || !Number.isFinite(amortization) || interest < 0 || amortization < 0) {
      errors.push(`Línea ${idx + 1}: fecha/interés/amortización inválidos.`);
      return;
    }
    rows.push({ date, interest, amortization });
  });
  rows.sort((a, b) => String(a.date).localeCompare(String(b.date)));
  return { rows, errors };
}

export function spreadMetrics(ask, bid) {
  ask = num(ask, NaN); bid = num(bid, NaN);
  if (!(ask > 0) || !(bid > 0)) return { abs: null, pctAsk: null, pctMid: null, inverted: false };
  // En un libro normal la punta compradora (bid) debe ser menor o igual a la vendedora (ask).
  // Si ocurre al revés, lo más probable es que las puntas se hayan cargado invertidas o sean de momentos distintos.
  if (bid > ask) return { abs: null, pctAsk: null, pctMid: null, inverted: true };
  const abs = ask - bid;
  const mid = (ask + bid) / 2;
  return { abs, pctAsk: abs / ask * 100, pctMid: mid > 0 ? abs / mid * 100 : null, inverted: false };
}

function quoteScaleValue(v) {
  const n = num(v, 0);
  return n > 0 ? n : 0;
}

function flowBaseValue(v) {
  const n = num(v, 0);
  return n > 0 ? n : 0;
}

function futureFlows(input, settlement) {
  return (input.flows || [])
    .map(f => ({ date: isoDate(f.date), interest: Math.max(0, num(f.interest)), amortization: Math.max(0, num(f.amortization)) }))
    .filter(f => f.date && asDate(f.date) > settlement)
    .sort((a, b) => a.date.localeCompare(b.date));
}

function currentFx(input) {
  const pc = String(input.paymentCurrency || '').toUpperCase();
  const qc = String(input.quoteCurrency || '').toUpperCase();
  if (pc && qc && pc === qc) return 1;
  const fx = num(input.fxRateQuotePerPayment, 0);
  return fx > 0 ? fx : null;
}

function exitFx(input) {
  const pc = String(input.paymentCurrency || '').toUpperCase();
  const qc = String(input.quoteCurrency || '').toUpperCase();
  if (pc && qc && pc === qc) return 1;
  const fx = num(input.exitFxRateQuotePerPayment, 0);
  return fx > 0 ? fx : null;
}

function dirtyQuotePrice(input, which = 'ask') {
  const p = num(which === 'bid' ? input.bidPrice : input.askPrice, NaN);
  if (!(p > 0)) return null;
  const accrued = String(input.priceType || 'dirty') === 'clean' ? Math.max(0, num(input.accruedInterest)) : 0;
  return p + accrued;
}

export function theoreticalPriceFromYield(input, annualYieldPct) {
  const settlement = asDate(input.settlementDate || input.quoteTimestamp || new Date());
  const qs = quoteScaleValue(input.quoteScale), fb = flowBaseValue(input.flowBase);
  const fx = currentFx(input);
  if (!settlement || !qs || !fb || !fx) return null;
  const flows = futureFlows(input, settlement);
  if (!flows.length) return null;
  const y = num(annualYieldPct, NaN) / 100;
  if (!(y > -1)) return null;
  let pvPaymentPerFlowBase = 0;
  for (const f of flows) {
    const yf = yearFraction(settlement, f.date, input.dayCount || 'ACT/365');
    pvPaymentPerFlowBase += (f.interest + f.amortization) / Math.pow(1 + y, yf);
  }
  let quoteDirty = pvPaymentPerFlowBase * fx * qs / fb;
  if (String(input.priceType || 'dirty') === 'clean') quoteDirty -= Math.max(0, num(input.accruedInterest));
  return Number.isFinite(quoteDirty) ? quoteDirty : null;
}

export function maxPurchasePriceForNetYield(input, annualYieldPct) {
  const theoretical = theoreticalPriceFromYield(input, annualYieldPct);
  if (theoretical == null) return null;
  const fee = Math.max(0, num(input.buyFeePct)) / 100;
  const accrued = String(input.priceType || 'dirty') === 'clean' ? Math.max(0, num(input.accruedInterest)) : 0;
  // theoreticalPriceFromYield devuelve el precio cotizado (limpio si corresponde).
  // Para aplicar comisión correctamente volvemos a precio sucio, descontamos el costo y regresamos a limpio.
  const dirty = theoretical + accrued;
  const maxDirtyAsk = dirty / (1 + fee);
  return String(input.priceType || 'dirty') === 'clean' ? maxDirtyAsk - accrued : maxDirtyAsk;
}

function fmtDecision(kind) {
  if (kind === 'attractive') return '🟢 OPORTUNIDAD ATRACTIVA';
  if (kind === 'wait') return '🟡 ESPERAR MEJOR PRECIO';
  if (kind === 'risk') return '🔴 RIESGO ELEVADO / DESCARTAR';
  return '⚪ INFORMACIÓN INSUFICIENTE';
}

function quoteAgeHours(ts) {
  const d = asDate(ts);
  if (!d) return null;
  return (Date.now() - d.getTime()) / 3600000;
}

export function analyzeFixedIncome(raw) {
  const input = { ...raw };
  const settlement = asDate(input.settlementDate || input.quoteTimestamp || new Date());
  const qs = quoteScaleValue(input.quoteScale), fb = flowBaseValue(input.flowBase);
  const fx = currentFx(input);
  const askDirty = dirtyQuotePrice(input, 'ask');
  const bidDirty = dirtyQuotePrice(input, 'bid');
  const buyFee = Math.max(0, num(input.buyFeePct)) / 100;
  const sellFee = Math.max(0, num(input.sellFeePct)) / 100;
  const convention = input.dayCount || 'ACT/365';
  const flows = settlement ? futureFlows(input, settlement) : [];
  const limitations = [], reasons = [], warnings = [];

  if (!qs) limitations.push('Falta definir la escala de cotización (por cuántos VN está expresado el precio).');
  if (!fb) limitations.push('Falta definir la base del calendario de flujos.');
  if (!(askDirty > 0)) limitations.push('Falta precio efectivo de compra.');
  if (!settlement) limitations.push('Falta fecha de liquidación/cotización válida.');
  if (!flows.length) limitations.push('Falta el calendario futuro de intereses y amortizaciones: no se calcula TIR exacta ni precio por rendimiento objetivo.');
  if (!fx) limitations.push('La moneda de cotización difiere de la de pago y falta el tipo de cambio de conversión.');

  const ageHours = quoteAgeHours(input.quoteTimestamp);
  if (ageHours == null) warnings.push('No se pudo verificar la antigüedad de la cotización.');
  else if (ageHours > 72) limitations.push(`Cotización desactualizada (${ageHours.toFixed(0)} h). No sirve para decidir si actuar ahora.`);
  else if (ageHours > 24) warnings.push(`Cotización con ${ageHours.toFixed(0)} h de antigüedad.`);

  const spread = spreadMetrics(input.askPrice, input.bidPrice);
  if (spread.inverted) limitations.push('Las puntas de compra/venta parecen invertidas o pertenecen a momentos distintos.');
  const maxSpread = Math.max(0.05, num(input.maxSpreadPct, 2));
  if (spread.inverted) {
    warnings.push('Las puntas parecen invertidas: el precio de VENDER (punta compradora) quedó por encima del precio de COMPRAR (punta vendedora). Revisá los dos valores antes de decidir.');
  } else if (spread.pctAsk != null) {
    if (spread.pctAsk > maxSpread * 2) warnings.push(`Spread muy alto (${spread.pctAsk.toFixed(2)}%): entrar y salir puede ser costoso.`);
    else if (spread.pctAsk > maxSpread) warnings.push(`Spread por encima de tu máximo (${spread.pctAsk.toFixed(2)}% > ${maxSpread.toFixed(2)}%).`);
    else reasons.push(`Spread dentro de tu límite (${spread.pctAsk.toFixed(2)}%).`);
  } else warnings.push('No hay dos puntas válidas para medir spread/liquidez.');

  let grossYtmPct = null, netYtmPct = null, purchasePaymentPerBase = null;
  let nominalPurchased = null, initialCostPayment = null, totalInterest = null, capitalRecovered = null, totalFuture = null, netGainMaturity = null, totalReturnPct = null, currentYieldPct = null;
  if (settlement && qs && fb && fx && askDirty > 0 && flows.length) {
    purchasePaymentPerBase = askDirty / qs * fb / fx;
    const grossFlows = [{ date: settlement.toISOString().slice(0, 10), amount: -purchasePaymentPerBase }, ...flows.map(f => ({ date: f.date, amount: f.interest + f.amortization }))];
    const netFlows = [{ date: settlement.toISOString().slice(0, 10), amount: -purchasePaymentPerBase * (1 + buyFee) }, ...flows.map(f => ({ date: f.date, amount: f.interest + f.amortization }))];
    const gy = xirr(grossFlows, convention), ny = xirr(netFlows, convention);
    grossYtmPct = gy == null ? null : gy * 100;
    netYtmPct = ny == null ? null : ny * 100;

    const invQuote = Math.max(0, num(input.investmentAmount));
    if (invQuote > 0) {
      const quoteCostPerNominal = askDirty / qs * (1 + buyFee);
      nominalPurchased = quoteCostPerNominal > 0 ? invQuote / quoteCostPerNominal : null;
      initialCostPayment = invQuote / fx;
      const scale = nominalPurchased / fb;
      totalInterest = flows.reduce((s, f) => s + f.interest, 0) * scale;
      capitalRecovered = flows.reduce((s, f) => s + f.amortization, 0) * scale;
      totalFuture = totalInterest + capitalRecovered;
      netGainMaturity = totalFuture - initialCostPayment;
      totalReturnPct = initialCostPayment > 0 ? netGainMaturity / initialCostPayment * 100 : null;
    }
    const oneYear = new Date(settlement.getTime() + 365 * 86400000);
    const interest12 = flows.filter(f => asDate(f.date) <= oneYear).reduce((s, f) => s + f.interest, 0);
    currentYieldPct = purchasePaymentPerBase > 0 ? interest12 / purchasePaymentPerBase * 100 : null;
  }

  const targetYieldPct = num(input.targetYieldPct, 0);
  const idealPrice = targetYieldPct > -99 ? maxPurchasePriceForNetYield(input, targetYieldPct) : null;
  if (targetYieldPct > 0 && netYtmPct != null) {
    if (netYtmPct >= targetYieldPct) reasons.push(`TIR neta ${netYtmPct.toFixed(2)}% alcanza tu objetivo de ${targetYieldPct.toFixed(2)}%.`);
    else warnings.push(`TIR neta ${netYtmPct.toFixed(2)}% queda debajo de tu objetivo de ${targetYieldPct.toFixed(2)}%.`);
  }
  if (idealPrice != null && num(input.askPrice) > 0) {
    const gap = (num(input.askPrice) / idealPrice - 1) * 100;
    if (gap <= 1) reasons.push(`El precio ofrecido es compatible con tu rendimiento objetivo (máximo aprox. ${idealPrice.toFixed(2)}).`);
    else warnings.push(`Para tu rendimiento objetivo, el precio máximo sería aprox. ${idealPrice.toFixed(2)}; la punta vendedora está ${gap.toFixed(2)}% arriba.`);
  }

  const creditRisk = String(input.creditRisk || 'desconocido').toLowerCase();
  if (creditRisk === 'alto') warnings.push('Riesgo crediticio marcado como ALTO.');
  else if (creditRisk === 'medio') reasons.push('Riesgo crediticio informado como MEDIO; exige prima de rendimiento acorde.');
  else if (creditRisk === 'bajo') reasons.push('Riesgo crediticio informado como BAJO.');
  else warnings.push('Riesgo crediticio no informado: no se habilita una señal verde de compra.');

  const nde = num(input.netDebtEbitda, 0), ic = num(input.interestCoverage, 0);
  if (String(input.type || '').toLowerCase().includes('on')) {
    if (nde > 6) warnings.push(`Deuda neta/EBITDA elevada (${nde.toFixed(1)}x).`);
    else if (nde > 0 && nde <= 4) reasons.push(`Deuda neta/EBITDA informada en zona manejable (${nde.toFixed(1)}x).`);
    if (ic > 0 && ic < 1.5) warnings.push(`Cobertura de intereses débil (${ic.toFixed(1)}x).`);
    else if (ic >= 2.5) reasons.push(`Cobertura de intereses informada sólida (${ic.toFixed(1)}x).`);
  }

  const dur = num(input.modifiedDuration || input.duration, 0);
  const maxDur = Math.max(0.5, num(input.maxDurationYears, 6));
  if (dur > 0) {
    if (dur > maxDur) warnings.push(`Duration ${dur.toFixed(2)} supera tu máximo ${maxDur.toFixed(2)}: mayor sensibilidad a tasas.`);
    else reasons.push(`Duration ${dur.toFixed(2)} dentro de tu límite.`);
  } else warnings.push('Duration no informada: la sensibilidad a tasas tiene menor precisión.');

  const balanzYtm = num(input.balanzYtmPct, 0);
  const ytmDiff = balanzYtm && grossYtmPct != null ? grossYtmPct - balanzYtm : null;
  if (ytmDiff != null && Math.abs(ytmDiff) > 0.35) warnings.push(`La TIR calculada difiere ${ytmDiff.toFixed(2)} p.p. de Balanz: revisar escala, precio limpio/sucio, FX y calendario.`);

  const severeCredit = creditRisk === 'alto' || (nde > 6) || (ic > 0 && ic < 1.5);
  const severeLiquidity = spread.pctAsk != null && spread.pctAsk > maxSpread * 2;
  const historical = Boolean(input.historicalExample);
  let kind = 'insufficient';
  if (historical) {
    limitations.push('Ejemplo histórico: sirve para probar fórmulas/interfaz, no para recomendar una compra actual.');
    kind = 'insufficient';
  } else if (limitations.length) {
    kind = 'insufficient';
  } else if (severeCredit || severeLiquidity || (netYtmPct != null && netYtmPct < 0)) {
    kind = 'risk';
  } else {
    const yieldOk = targetYieldPct > 0 && netYtmPct != null && netYtmPct >= targetYieldPct;
    const spreadOk = spread.pctAsk != null && spread.pctAsk <= maxSpread;
    const creditOk = creditRisk === 'bajo' || creditRisk === 'medio';
    const priceOk = idealPrice != null && num(input.askPrice) <= idealPrice * 1.01;
    const durationOk = !dur || dur <= maxDur;
    const fresh = ageHours != null && ageHours <= 24;
    kind = yieldOk && spreadOk && creditOk && priceOk && durationOk && fresh ? 'attractive' : 'wait';
  }

  const completeness = [flows.length > 0, !!fx, spread.pctAsk != null, creditRisk !== 'desconocido', dur > 0, ageHours != null].filter(Boolean).length;
  const confidence = limitations.length ? 'Baja' : completeness >= 6 ? 'Alta' : completeness >= 4 ? 'Media' : 'Baja';

  const sensitivity = [];
  const baseYield = balanzYtm || grossYtmPct;
  if (baseYield != null && Number.isFinite(baseYield) && flows.length && fx) {
    for (const delta of [-2, -1, 0, 1, 2]) {
      const y = Math.max(-99, baseYield + delta);
      const price = theoreticalPriceFromYield(input, y);
      const pnlPct = price != null && num(input.askPrice) > 0 ? (price / num(input.askPrice) - 1) * 100 : null;
      sensitivity.push({ deltaPctPoints: delta, yieldPct: y, price, pnlPct });
    }
  }

  const sale = analyzeSaleScenario(input, { settlement, qs, fb, fx, flows, askDirty, buyFee, sellFee });
  const manualAlerts = evaluateManualAlerts(input, { netYtmPct, spreadPct: spread.pctAsk });

  return {
    input,
    exact: limitations.length === 0,
    decisionKind: kind,
    decision: fmtDecision(kind),
    confidence,
    limitations,
    warnings,
    reasons,
    spread,
    quoteAgeHours: ageHours,
    grossYtmPct,
    netYtmPct,
    balanzYtmPct: balanzYtm || null,
    ytmDiffPctPoints: ytmDiff,
    currentYieldPct,
    idealPrice,
    purchasePaymentPerBase,
    nominalPurchased,
    initialCostPayment,
    totalInterest,
    capitalRecovered,
    totalFuture,
    netGainMaturity,
    totalReturnPct,
    sensitivity,
    sale,
    manualAlerts,
  };
}

export function analyzeSaleScenario(input, ctx = {}) {
  const settlement = ctx.settlement || asDate(input.settlementDate || input.quoteTimestamp || new Date());
  const saleDate = asDate(input.saleDate);
  const qs = ctx.qs || quoteScaleValue(input.quoteScale), fb = ctx.fb || flowBaseValue(input.flowBase);
  const fx0 = ctx.fx || currentFx(input), fx1 = exitFx(input);
  const askDirty = ctx.askDirty || dirtyQuotePrice(input, 'ask');
  const target = num(input.targetSellPrice, 0);
  const buyFee = ctx.buyFee ?? Math.max(0, num(input.buyFeePct)) / 100;
  const sellFee = ctx.sellFee ?? Math.max(0, num(input.sellFeePct)) / 100;
  if (!settlement || !saleDate || saleDate <= settlement || !qs || !fb || !fx0 || !fx1 || !(askDirty > 0) || !(target > 0)) {
    return { available: false, reason: 'Para simular venta anticipada necesitás fecha de venta, precio objetivo y FX de salida si las monedas difieren.' };
  }
  const flows = ctx.flows || futureFlows(input, settlement);
  const interim = flows.filter(f => asDate(f.date) < saleDate);
  const costBase = askDirty / qs * fb / fx0 * (1 + buyFee);
  // El precio objetivo de venta se interpreta como precio efectivo/sucio: no reutilizamos corridos de hoy en una fecha futura.
  const targetDirty = target;
  const saleBase = targetDirty / qs * fb / fx1 * (1 - sellFee);
  const irrFlows = [{ date: settlement.toISOString().slice(0, 10), amount: -costBase }, ...interim.map(f => ({ date: f.date, amount: f.interest + f.amortization })), { date: saleDate.toISOString().slice(0, 10), amount: saleBase }];
  const r = xirr(irrFlows, input.dayCount || 'ACT/365');
  const interimSum = interim.reduce((s, f) => s + f.interest + f.amortization, 0);
  const profitBase = interimSum + saleBase - costBase;
  const returnPct = costBase > 0 ? profitBase / costBase * 100 : null;
  const neededSalePaymentBase = Math.max(0, costBase - interimSum) / Math.max(EPS, 1 - sellFee);
  const breakEvenQuote = neededSalePaymentBase * fx1 * qs / fb;
  return { available: true, annualizedPct: r == null ? null : r * 100, profitPerFlowBase: profitBase, returnPct, breakEvenSalePrice: breakEvenQuote, interimCashPerFlowBase: interimSum };
}

export function evaluateManualAlerts(input, ctx = {}) {
  const out = [];
  const targetPrice = num(input.alertPriceMax, 0);
  const minYield = num(input.alertYieldMinPct, 0);
  const maxSpread = num(input.alertSpreadMaxPct, 0);
  const ask = num(input.askPrice, 0);
  if (targetPrice > 0) out.push({ type: 'precio', configured: true, hit: ask > 0 && ask <= targetPrice, text: `Precio ≤ ${targetPrice}` });
  if (minYield > 0) out.push({ type: 'tir', configured: true, hit: ctx.netYtmPct != null && ctx.netYtmPct >= minYield, text: `TIR neta ≥ ${minYield}%` });
  if (maxSpread > 0) out.push({ type: 'spread', configured: true, hit: ctx.spreadPct != null && ctx.spreadPct <= maxSpread, text: `Spread ≤ ${maxSpread}%` });
  return out.map(x => ({ ...x, verification: 'MANUAL / NO VERIFICADA EN TIEMPO REAL' }));
}

export function al30Example() {
  return {
    ticker: 'AL30', issuer: 'República Argentina', type: 'Bono soberano', paymentCurrency: 'USD', quoteCurrency: 'ARS', species: 'Pesos',
    askPrice: 853.50, bidPrice: '', quoteTimestamp: '2026-10-16T16:00', investmentAmount: 1000000,
    settlementDate: '2026-10-16', maturityDate: '2030-07-09', couponAnnualPct: 0.75, frequency: 2, amortizationType: 'Parcial', residualPct: 64,
    quoteScale: 1, flowBase: 100, priceType: 'dirty', accruedInterest: 0, dayCount: '30/360', nextPaymentDate: '2027-01-09', balanzYtmPct: 9.35, duration: 1.69,
    targetYieldPct: 10, maxSpreadPct: 2, maxDurationYears: 6, creditRisk: 'alto', historicalExample: true,
    notes: 'Ejemplo histórico. AL30 amortiza el residual en cuotas semestrales; falta ingresar MEP y punta compradora de esa pantalla para comparar exactamente la TIR en USD.',
    flows: [
      {date:'2027-01-09', interest:0.24, amortization:8},
      {date:'2027-07-09', interest:0.21, amortization:8},
      {date:'2028-01-09', interest:0.42, amortization:8},
      {date:'2028-07-09', interest:0.35, amortization:8},
      {date:'2029-01-09', interest:0.28, amortization:8},
      {date:'2029-07-09', interest:0.21, amortization:8},
      {date:'2030-01-09', interest:0.14, amortization:8},
      {date:'2030-07-09', interest:0.07, amortization:8},
    ]
  };
}

export function lqc1dExample() {
  return {
    ticker: 'LQC1D', issuer: 'Petroquímica Comodoro Rivadavia / Luz de Tres Picos', type: 'ON corporativa', paymentCurrency: 'USD', quoteCurrency: 'USD', species: 'Dólar MEP',
    askPrice: 1027, bidPrice: '', quoteTimestamp: '2026-10-16T16:00', investmentAmount: 5000,
    settlementDate: '2026-10-16', maturityDate: '2034-07-28', couponAnnualPct: 8.5, frequency: 2, amortizationType: '3 cuotas: 33%, 33%, 34%', residualPct: 100,
    quoteScale: 1000, flowBase: 1000, priceType: 'clean', accruedInterest: 0, dayCount: '30/360', nextPaymentDate: '2027-01-28', balanzYtmPct: 8.51, duration: 4.78,
    targetYieldPct: 9, maxSpreadPct: 2, maxDurationYears: 6, creditRisk: 'medio', historicalExample: true,
    notes: 'Ejemplo histórico. La estructura publicada es amortizante en tres cuotas (33%, 33% y 34%). Cargá el calendario exacto del suplemento/Balanz antes de calcular la TIR propia.',
    flows: []
  };
}
