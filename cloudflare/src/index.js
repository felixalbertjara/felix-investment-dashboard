const DEFAULTS = {
  portfolio: { positions: [] },
  alert_state: {},
  alert_history: [],
  last_prices: { prices: {} },
  buy_alerts_state: {},
  portfolio_snapshot: null,
  market_snapshot: null,
};

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);
    if (url.pathname.startsWith('/internal/')) {
      if (!internalAuthorized(request, env)) return json({ error: 'unauthorized' }, 401);
      return handleInternal(request, env, url);
    }
    if (!basicAuthorized(request, env)) {
      return new Response('Autenticación requerida', {
        status: 401,
        headers: { 'WWW-Authenticate': 'Basic realm="Felix Dashboard"' },
      });
    }
    if (url.pathname.startsWith('/api/')) return handleApi(request, env, url, ctx);
    return env.ASSETS.fetch(request);
  },

  async scheduled(controller, env, ctx) {
    ctx.waitUntil(handleCron(controller, env));
  },
};

function json(data, status = 200) {
  return new Response(JSON.stringify(data), {
    status,
    headers: {
      'content-type': 'application/json; charset=utf-8',
      'cache-control': 'no-store',
    },
  });
}

function internalAuthorized(request, env) {
  const auth = request.headers.get('authorization') || '';
  return !!env.SCANNER_SECRET && auth === `Bearer ${env.SCANNER_SECRET}`;
}

function basicAuthorized(request, env) {
  if (!env.DASHBOARD_USER || !env.DASHBOARD_PASSWORD) return false;
  const header = request.headers.get('authorization') || '';
  if (!header.startsWith('Basic ')) return false;
  try {
    const decoded = atob(header.slice(6));
    const i = decoded.indexOf(':');
    if (i < 0) return false;
    return timingSafeEqual(decoded.slice(0, i), env.DASHBOARD_USER) &&
      timingSafeEqual(decoded.slice(i + 1), env.DASHBOARD_PASSWORD);
  } catch {
    return false;
  }
}

function timingSafeEqual(a, b) {
  if (typeof a !== 'string' || typeof b !== 'string' || a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
  return diff === 0;
}

async function handleInternal(request, env, url) {
  if (request.method === 'GET' && url.pathname === '/internal/market-status') {
    const raw = url.searchParams.get('at');
    const when = raw ? new Date(raw) : new Date();
    if (Number.isNaN(when.getTime())) return json({ error: 'at inválido' }, 400);
    return json(await marketStatus(env.DB, when));
  }


  if (request.method === 'PUT' && url.pathname.startsWith('/internal/snapshot/')) {
    const kind = url.pathname.slice('/internal/snapshot/'.length);
    const key = kind === 'portfolio' ? 'portfolio_snapshot' : kind === 'market' ? 'market_snapshot' : '';
    if (!key) return json({ error: 'snapshot inválido' }, 404);
    const declared = Number(request.headers.get('content-length') || 0);
    if (declared > 24 * 1024 * 1024) return json({ error: 'snapshot demasiado grande' }, 413);
    // Se guarda el JSON crudo en KV: no lo parseamos/serializamos en el Worker.
    // Esto ahorra CPU y evita el límite de 2 MB por valor/fila de D1.
    const raw = await request.text();
    if (raw.length > 24 * 1024 * 1024) return json({ error: 'snapshot demasiado grande' }, 413);
    const updated = new Date().toISOString();
    await env.SNAPSHOTS.put(key, raw, { metadata: { version: updated } });
    await setMeta(env.DB, `${key}_updated_at`, updated);
    return json({ ok: true, key, chars: raw.length, updated_at: updated });
  }

  if (request.method === 'GET' && url.pathname === '/internal/export') {
    const out = {};
    const p = await getBlobWithVersion(env.DB, 'portfolio');
    const h = await getBlobWithVersion(env.DB, 'alert_history');
    out.portfolio = p.value;
    out.portfolio_version = p.updated_at || '';
    out.alert_history = h.value;
    out.alert_history_version = h.updated_at || '';
    for (const key of ['alert_state','last_prices','buy_alerts_state']) {
      out[key] = await getBlob(env.DB, key);
    }
    return json(out);
  }

  if (request.method === 'POST' && url.pathname === '/internal/import') {
    let body;
    try { body = await request.json(); } catch { return json({ error: 'json inválido' }, 400); }
    const allowed = ['alert_state','last_prices','buy_alerts_state'];
    let planApplied = false;
    if (Array.isArray(body.portfolio_plan_updates) && body.portfolio_plan_updates.length) {
      const current = await getBlobWithVersion(env.DB, 'portfolio');
      if (!body.portfolio_base_version || body.portfolio_base_version === current.updated_at) {
        const store = current.value || {positions:[]};
        if (!Array.isArray(store.positions)) store.positions = [];
        const fields = ['strategy','expected_days','initial_stop_price','initial_risk_pct','stop_source','entry_atr','entry_support','entry_rsi','entry_score','entry_underlying_score','entry_ccl_proxy','plan_created_at'];
        for (const upd of body.portfolio_plan_updates) {
          const t = String(upd.ticker || '').toUpperCase();
          const p = store.positions.find(x => String(x.ticker || '').toUpperCase() === t);
          if (!p) continue;
          for (const f of fields) if (f in upd) p[f] = upd[f];
          if (upd.plan_created_at && !String(upd.plan_created_at).startsWith('0001-')) p.updated_at = upd.updated_at || new Date().toISOString();
        }
        await putBlob(env.DB, 'portfolio', store);
        planApplied = true;
      }
    }
    let historyApplied = false;
    if ('alert_history' in body && body.alert_history != null) {
      const currentHistory = await getBlobWithVersion(env.DB, 'alert_history');
      if (!body.alert_history_base_version || body.alert_history_base_version === currentHistory.updated_at) {
        await putBlob(env.DB, 'alert_history', body.alert_history);
        historyApplied = true;
      }
    }
    let saved = 0;
    for (const key of allowed) {
      if (!(key in body) || body[key] == null) continue;
      await putBlob(env.DB, key, body[key]);
      saved++;
    }
    if (body.scan_mode) await setMeta(env.DB, 'last_scan_mode', String(body.scan_mode));
    if (body.scan_at) await setMeta(env.DB, 'last_scan_at', String(body.scan_at));
    return json({ ok: true, saved, plan_applied: planApplied, history_applied: historyApplied });
  }

  return json({ error: 'not found' }, 404);
}

async function handleApi(request, env, url, ctx) {
  if (request.method === 'GET' && url.pathname === '/api/dashboard') {
    const [portfolio, alertHistory, lastScanAt, lastScanMode, marketSnapshotAt, portfolioSnapshotAt] = await Promise.all([
      getBlob(env.DB, 'portfolio'),
      getBlob(env.DB, 'alert_history'),
      getMeta(env.DB, 'last_scan_at'),
      getMeta(env.DB, 'last_scan_mode'),
      getMeta(env.DB, 'market_snapshot_updated_at'),
      getMeta(env.DB, 'portfolio_snapshot_updated_at'),
    ]);
    const status = await marketStatus(env.DB, new Date());
    return json({
      portfolio_raw: portfolio || DEFAULTS.portfolio,
      alert_history: alertHistory || [],
      market_status: status,
      last_scan_at: lastScanAt,
      last_scan_mode: lastScanMode,
      market_snapshot_updated_at: marketSnapshotAt,
      portfolio_snapshot_updated_at: portfolioSnapshotAt,
    });
  }

  if (request.method === 'GET' && url.pathname === '/api/snapshot/market') {
    const item = await env.SNAPSHOTS.getWithMetadata('market_snapshot');
    return new Response(item?.value || '{}', { headers: { 'content-type':'application/json; charset=utf-8', 'cache-control':'no-store', 'x-snapshot-version':String(item?.metadata?.version || '') } });
  }

  if (request.method === 'GET' && url.pathname === '/api/snapshot/portfolio') {
    const item = await env.SNAPSHOTS.getWithMetadata('portfolio_snapshot');
    return new Response(item?.value || '{}', { headers: { 'content-type':'application/json; charset=utf-8', 'cache-control':'no-store', 'x-snapshot-version':String(item?.metadata?.version || '') } });
  }

  if (request.method === 'POST' && url.pathname === '/api/portfolio/save') {
    let body;
    try { body = await request.json(); } catch { return json({ error: 'json inválido' }, 400); }
    const result = await savePortfolio(env, body);
    if (result.error) return json(result, 400);
    const status = await marketStatus(env.DB, new Date());
    if (status.open) ctx.waitUntil(dispatchWorkflow(env, 'portfolio.yml', { reason: 'portfolio-save', scheduled_at: new Date().toISOString() }).catch(()=>{}));
    return json({ ok: true, portfolio: result.portfolio, plan_pending: true });
  }

  if (request.method === 'POST' && url.pathname === '/api/portfolio/delete') {
    let body;
    try { body = await request.json(); } catch { return json({ error: 'json inválido' }, 400); }
    const ticker = String(body.ticker || '').trim().toUpperCase();
    if (!ticker) return json({ error: 'ticker requerido' }, 400);
    const store = (await getBlob(env.DB, 'portfolio')) || DEFAULTS.portfolio;
    store.positions = (store.positions || []).filter(p => String(p.ticker || '').toUpperCase() !== ticker);
    await putBlob(env.DB, 'portfolio', store);
    return json({ ok: true });
  }

  if (request.method === 'POST' && url.pathname === '/api/portfolio/import') {
    let body;
    try { body = await request.json(); } catch { return json({ error: 'json inválido' }, 400); }
    if (!body || !Array.isArray(body.positions)) return json({ error: 'portfolio.json inválido' }, 400);
    await putBlob(env.DB, 'portfolio', body);
    return json({ ok: true, positions: body.positions.length });
  }

  if (request.method === 'POST' && url.pathname === '/api/alerts/clear') {
    await putBlob(env.DB, 'alert_history', []);
    // Importante: NO tocamos alert_state para conservar anti-spam / confirmaciones.
    return json({ ok: true });
  }

  if (request.method === 'POST' && url.pathname === '/api/test-alert') {
    ctx.waitUntil(dispatchWorkflow(env, 'test-alert.yml', { reason: 'dashboard-button' }));
    return json({ ok: true, message: 'Prueba enviada a GitHub Actions' });
  }

  if (request.method === 'POST' && url.pathname === '/api/refresh') {
    return json({ ok: true, message: 'La vista se recarga desde el último snapshot; no genera tráfico de mercado.' });
  }

  return json({ error: 'not found' }, 404);
}

async function savePortfolio(env, body) {
  const ticker = String(body.ticker || '').trim().toUpperCase();
  const mode = String(body.mode || 'replace').trim();
  const amount = Number(body.amount_ars || 0);
  const price = Number(body.avg_buy_price || 0);
  const buyDate = String(body.buy_date || '').trim() || artDate(new Date());
  const notes = String(body.notes || '').trim();
  const strategy = String(body.strategy || 'SWING').toUpperCase() === 'POSITION' ? 'POSITION' : 'SWING';
  const expectedDays = Math.max(5, Math.min(365, Number(body.expected_days || (strategy === 'POSITION' ? 120 : 45))));
  const manualStop = Number(body.initial_stop_price || 0);
  if (!ticker || !(amount > 0) || !(price > 0)) return { error: 'ticker, ARS y precio son obligatorios' };

  const store = (await getBlob(env.DB, 'portfolio')) || { positions: [] };
  if (!Array.isArray(store.positions)) store.positions = [];
  const now = new Date().toISOString();
  const idx = store.positions.findIndex(p => String(p.ticker || '').toUpperCase() === ticker);

  const resetPlan = p => {
    p.strategy = strategy;
    p.expected_days = expectedDays;
    p.initial_stop_price = manualStop > 0 ? manualStop : 0;
    p.initial_risk_pct = manualStop > 0 && price > 0 ? Math.max(0, (price - manualStop) / price * 100) : 0;
    p.stop_source = manualStop > 0 ? 'manual' : 'plan pendiente de scanner';
    p.entry_atr = 0; p.entry_support = 0; p.entry_rsi = 0; p.entry_score = 0;
    p.entry_underlying_score = 0; p.entry_ccl_proxy = 0; p.plan_created_at = '0001-01-01T00:00:00Z';
  };

  if (idx >= 0) {
    const p = store.positions[idx];
    if (mode === 'add') {
      const oldQty = Number(p.avg_buy_price || 0) > 0 ? Number(p.amount_ars || 0) / Number(p.avg_buy_price) : 0;
      const newQty = amount / price;
      const newAmount = Number(p.amount_ars || 0) + amount;
      const newAvg = oldQty + newQty > 0 ? ((oldQty * Number(p.avg_buy_price || 0)) + (newQty * price)) / (oldQty + newQty) : price;
      p.amount_ars = newAmount;
      p.avg_buy_price = newAvg;
      if (notes) p.notes = `${String(p.notes || '').trim()}${p.notes ? ' | ' : ''}Compra adicional ${artDateDisplay(new Date())}: ${notes}`;
    } else {
      p.amount_ars = amount;
      p.avg_buy_price = price;
      p.buy_date = buyDate;
      p.notes = notes;
    }
    resetPlan(p);
    p.updated_at = now;
  } else {
    const p = {
      ticker, amount_ars: amount, avg_buy_price: price, buy_date: buyDate, notes,
      strategy, expected_days: expectedDays, initial_stop_price: 0, initial_risk_pct: 0,
      stop_source: 'plan pendiente de scanner', entry_atr: 0, entry_support: 0, entry_rsi: 0,
      entry_score: 0, entry_underlying_score: 0, entry_ccl_proxy: 0,
      plan_created_at: '0001-01-01T00:00:00Z', created_at: now, updated_at: now,
    };
    resetPlan(p);
    store.positions.push(p);
  }
  await putBlob(env.DB, 'portfolio', store);
  return { portfolio: store };
}

async function handleCron(controller, env) {
  const when = controller?.scheduledTime ? new Date(controller.scheduledTime) : new Date();
  const status = await marketStatus(env.DB, when);
  if (!status.open) return;
  const local = artParts(when);
  const minute = Number(local.minute);
  await dispatchWorkflow(env, 'portfolio.yml', { reason: 'cron', slot: `${local.date} ${local.hour}:${local.minute}`, scheduled_at: when.toISOString() });
  if (minute % 15 === 0) {
    await dispatchWorkflow(env, 'radar.yml', { reason: 'cron', slot: `${local.date} ${local.hour}:${local.minute}`, scheduled_at: when.toISOString() });
  }
  await setMeta(env.DB, 'last_dispatch_at', when.toISOString());
}

async function marketStatus(_db, when) {
  const p = artParts(when);
  const h = Number(p.hour), m = Number(p.minute), weekday = Number(p.weekdayNum);
  const mins = h * 60 + m;
  if (weekday === 0 || weekday === 6) return { open: false, reason: 'fin de semana', date: p.date, time: `${p.hour}:${p.minute}` };
  if (mins < 11*60 || mins > 17*60) return { open: false, reason: 'fuera de 11:00-17:00 ART', date: p.date, time: `${p.hour}:${p.minute}` };
  return { open: true, reason: 'lunes a viernes · ventana 11:00-17:00 ART', date: p.date, time: `${p.hour}:${p.minute}` };
}

function artParts(date) {
  const f = new Intl.DateTimeFormat('en-CA', {
    timeZone: 'America/Argentina/Buenos_Aires', year:'numeric', month:'2-digit', day:'2-digit',
    hour:'2-digit', minute:'2-digit', hourCycle:'h23', weekday:'short'
  });
  const o = Object.fromEntries(f.formatToParts(date).filter(x => x.type !== 'literal').map(x => [x.type, x.value]));
  const weekdayNum = ({Sun:0,Mon:1,Tue:2,Wed:3,Thu:4,Fri:5,Sat:6})[o.weekday];
  return { date:`${o.year}-${o.month}-${o.day}`, hour:o.hour, minute:o.minute, weekdayNum };
}
function artDate(d) { return artParts(d).date; }
function artDateDisplay(d) { const p=artParts(d); return `${p.date.slice(8,10)}/${p.date.slice(5,7)}/${p.date.slice(0,4)}`; }

async function dispatchWorkflow(env, workflow, inputs = {}) {
  if (!env.GITHUB_TOKEN || !env.GITHUB_OWNER || !env.GITHUB_REPO) throw new Error('GitHub dispatch no configurado');
  const endpoint = `https://api.github.com/repos/${env.GITHUB_OWNER}/${env.GITHUB_REPO}/actions/workflows/${workflow}/dispatches`;
  const r = await fetch(endpoint, {
    method:'POST',
    headers:{
      'authorization':`Bearer ${env.GITHUB_TOKEN}`,
      'accept':'application/vnd.github+json',
      'x-github-api-version':'2022-11-28',
      'user-agent':'FelixDashboardV3-Cloudflare',
      'content-type':'application/json',
    },
    body:JSON.stringify({ ref: env.GITHUB_BRANCH || 'main', inputs })
  });
  if (!r.ok) throw new Error(`GitHub dispatch ${workflow}: ${r.status} ${await r.text()}`);
}

async function getBlobWithVersion(db, key) {
  const row = await db.prepare('SELECT payload,updated_at FROM state_blobs WHERE key=?').bind(key).first();
  if (!row?.payload) return { value: structuredClone(DEFAULTS[key] ?? null), updated_at: row?.updated_at || '' };
  try {
    const parsed = JSON.parse(row.payload);
    if (parsed && Number(parsed.__chunked) > 0) {
      const parts = await db.prepare("SELECT key,payload FROM state_blobs WHERE key LIKE ? ORDER BY key").bind(`${key}:%`).all();
      const text = (parts.results || []).map(x => x.payload || '').join('');
      return { value: JSON.parse(text), updated_at: row.updated_at || '' };
    }
    return { value: parsed, updated_at: row.updated_at || '' };
  } catch {
    return { value: structuredClone(DEFAULTS[key] ?? null), updated_at: row.updated_at || '' };
  }
}

async function getBlob(db, key) {
  return (await getBlobWithVersion(db, key)).value;
}

function splitPayload(text, maxChars = 400000) {
  const out = [];
  for (let i = 0; i < text.length;) {
    let end = Math.min(text.length, i + maxChars);
    if (end < text.length) {
      const c = text.charCodeAt(end - 1);
      if (c >= 0xD800 && c <= 0xDBFF) end--;
    }
    out.push(text.slice(i, end));
    i = end;
  }
  return out;
}

async function putBlob(db, key, value) {
  const payload = JSON.stringify(value);
  const now = new Date().toISOString();
  // D1 limita cada string/fila a 2 MB. El radar completo puede superar eso,
  // así que lo troceamos; el resto normalmente queda en una sola fila.
  if (payload.length <= 400000) {
    await db.batch([
      db.prepare('DELETE FROM state_blobs WHERE key LIKE ?').bind(`${key}:%`),
      db.prepare(`INSERT INTO state_blobs(key,payload,updated_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at`).bind(key, payload, now),
    ]);
    return;
  }
  const chunks = splitPayload(payload);
  const stmts = [
    db.prepare('DELETE FROM state_blobs WHERE key LIKE ?').bind(`${key}:%`),
    db.prepare(`INSERT INTO state_blobs(key,payload,updated_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at`).bind(key, JSON.stringify({__chunked:chunks.length}), now),
  ];
  chunks.forEach((part, i) => {
    const ck = `${key}:${String(i).padStart(4,'0')}`;
    stmts.push(db.prepare(`INSERT INTO state_blobs(key,payload,updated_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at`).bind(ck, part, now));
  });
  await db.batch(stmts);
}

async function getMeta(db, key) {
  const row = await db.prepare('SELECT value FROM meta WHERE key=?').bind(key).first();
  return row?.value || '';
}
async function setMeta(db, key, value) {
  await db.prepare(`INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`).bind(key, value).run();
}
