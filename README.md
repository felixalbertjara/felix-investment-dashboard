# Felix Investment Dashboard V3 — Serverless $0

V3 conserva la lógica de trading de V2 (RSI, Stoch RSI, EMA20/50/200, MACD, ATR, volumen, soporte/resistencia, diario + 4H, subyacente USA + CCL proxy, R-multiple, stop técnico, profit lock, trailing, velocidad de retorno y time stop), pero ya no necesita una PC ni una VM encendida.

## Arquitectura

- **Cloudflare Worker + Static Assets**: dashboard web protegido con usuario/contraseña y API mínima.
- **Cloudflare D1**: estado pequeño y consistente: cartera, anti-spam, historial y cache liviano.
- **Cloudflare Workers KV**: snapshots grandes del radar y la cartera; evita inflar D1.
- **Cloudflare Cron**: reloj. Solo autoriza scans en la ventana acordada.
- **GitHub Actions**: ejecuta el scanner Go pesado sólo cuando Cloudflare lo llama.
- **Telegram**: alertas de compra confirmada, cancelación y salida/protección de posiciones.

La web nunca consulta Yahoo por sí sola: lee snapshots. Por eso abrir el dashboard fuera de horario no genera tráfico financiero.

## Horario exacto

Zona `America/Argentina/Buenos_Aires`.

- Lunes a viernes únicamente.
- Cartera: cada 5 minutos entre 11:00 y 17:00 ART inclusive.
- Radar: cada 15 minutos entre 11:00 y 17:00 ART inclusive.
- Sábados y domingos: 0 scans.
- Fuera de 11:00–17:00: 0 consultas a Yahoo/Google News.
- La web sigue online y muestra el último snapshot almacenado.

Cron UTC:

- `*/5 14-19 * * 1-5` → 11:00 a 16:55 ART.
- `0 20 * * 1-5` → 17:00 ART exactas.

Hay doble barrera: el cron limita el horario y el Worker vuelve a validar lunes-viernes + 11:00-17:00 ART antes de disparar GitHub. El runner Go también pregunta al Worker antes de consultar mercado.

## Regla de días

No usa calendario BYMA ni lista anual de feriados. La regla es deliberadamente simple y permanente: **lunes a viernes, 11:00 a 17:00 hora Argentina**. Por lo tanto no hay nada que actualizar al cambiar de año. Si un feriado cae de lunes a viernes, el sistema igualmente intenta sus scans dentro de esa ventana; si la fuente de mercado no publica datos nuevos, el scanner conservará/mostrará la información disponible y no se habilita ningún proceso fuera del horario configurado.

## Seguridad

Nunca se suben credenciales al repo.

Cloudflare Secrets:
- `DASHBOARD_USER`
- `DASHBOARD_PASSWORD`
- `SCANNER_SECRET`
- `GITHUB_TOKEN`

GitHub Repository Secrets:
- `DASHBOARD_API_URL`
- `SCANNER_SECRET`
- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_CHAT_ID`

El token/chat de Telegram sólo existen como GitHub Secrets. No están en el ZIP ni en el código.

## Botón borrar alertas

La interfaz agrega **🗑 Borrar alertas pasadas**. Borra únicamente `alert_history`; conserva `alert_state`, así no rompe el anti-spam ni reaparecen alertas antiguas como nuevas.

## Eficiencia

- El scan de cartera es independiente del radar.
- El radar se ejecuta 1 vez cada 15 min, no cada 5.
- `last_prices` persiste sólo precio/metadatos, no cientos de velas por activo.
- El estado de compra no guarda filas livianas inútiles.
- Snapshots grandes van a KV, no a D1.
- La web primero consulta metadatos; sólo vuelve a descargar un snapshot si su versión cambió.
- Fuera de horario no hay polling de mercado.

## Estructura

```
cloudflare/             Worker, frontend, D1, KV binding y cron
scanner/                motor Go derivado de V2
.github/workflows/      scanner cartera, radar y prueba Telegram
scripts/                utilidades de configuración
```

## Migrar la cartera de Windows

Cuando la V3 esté online, usá **Importar portfolio.json** y elegí el `data/portfolio.json` actual. No subas `telegram_secrets.txt`: el cloud usa Secrets.

## Importante

El sistema no envía órdenes a Balanz. Las señales son reglas cuantitativas/técnicas y no garantizan rentabilidad. Validá spread, liquidez y tamaño de posición antes de ejecutar.
