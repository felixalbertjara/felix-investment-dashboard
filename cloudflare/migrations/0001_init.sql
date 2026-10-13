CREATE TABLE IF NOT EXISTS state_blobs (
  key TEXT PRIMARY KEY,
  payload TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

-- Estado inicial. Cada blob equivale a uno de los JSON de la versión Windows.
INSERT OR IGNORE INTO state_blobs(key,payload,updated_at) VALUES
('portfolio','{"positions":[]}',CURRENT_TIMESTAMP),
('alert_state','{}',CURRENT_TIMESTAMP),
('alert_history','[]',CURRENT_TIMESTAMP),
('last_prices','{"prices":{}}',CURRENT_TIMESTAMP),
('buy_alerts_state','{}',CURRENT_TIMESTAMP);
