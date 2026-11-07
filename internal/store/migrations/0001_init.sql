CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'member',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Box-wide settings (title, setup flag); everything else is personal.
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL
);


-- ---------- Pocket CRM: everything below is per person ----------

-- The steps of a person's pipeline, in order. kind says what a deal in
-- this step is: still open, won or lost. probability is the default chance
-- (0–100) a deal here is won, for the weighted forecast.
CREATE TABLE stages (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  kind        TEXT NOT NULL DEFAULT 'open',            -- open | won | lost
  position    INTEGER NOT NULL DEFAULT 0,
  probability INTEGER NOT NULL DEFAULT 0,
  color       TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX stages_user ON stages(user_id, position);

-- A deal: possible work for someone, from the first talk to signed (or not).
-- The person and company are kept here as text so the app works without
-- Pocket Contacts; contact_ref / company_ref point at the cards there.
CREATE TABLE deals (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id          INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title            TEXT NOT NULL DEFAULT '',
  stage_id         INTEGER NOT NULL REFERENCES stages(id),
  status           TEXT NOT NULL DEFAULT 'open',          -- open | won | lost (the stage's kind)
  value            INTEGER NOT NULL DEFAULT 0,            -- in cents
  currency         TEXT NOT NULL DEFAULT 'EUR',
  probability      INTEGER NOT NULL DEFAULT -1,           -- -1 = the stage's
  expected_close   TEXT NOT NULL DEFAULT '',              -- YYYY-MM-DD
  source           TEXT NOT NULL DEFAULT '',              -- referral | website | form | email | event | social | repeat | other
  contact_name     TEXT NOT NULL DEFAULT '',
  contact_email    TEXT NOT NULL DEFAULT '',
  contact_phone    TEXT NOT NULL DEFAULT '',
  contact_ref      TEXT NOT NULL DEFAULT '',              -- Pocket Contacts id
  company          TEXT NOT NULL DEFAULT '',
  company_ref      TEXT NOT NULL DEFAULT '',              -- Pocket Contacts id of the company card
  description      TEXT NOT NULL DEFAULT '',
  next_action      TEXT NOT NULL DEFAULT '',
  next_due         TEXT NOT NULL DEFAULT '',              -- YYYY-MM-DD
  next_task_ref    TEXT NOT NULL DEFAULT '',              -- the same step as a task in Pocket Tasks
  tags             TEXT NOT NULL DEFAULT '[]',
  starred          INTEGER NOT NULL DEFAULT 0,
  position         REAL NOT NULL DEFAULT 0,               -- order within the stage
  lost_reason      TEXT NOT NULL DEFAULT '',
  outcome_note     TEXT NOT NULL DEFAULT '',
  closed_at        TEXT NOT NULL DEFAULT '',
  stage_changed_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  last_activity_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  source_app       TEXT NOT NULL DEFAULT '',              -- the app that made it through a Core link
  source_ref       TEXT NOT NULL DEFAULT '',              -- e.g. forms:answer:12
  client_key       TEXT NOT NULL DEFAULT '',              -- imports: the same row is never added twice
  created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX deals_user ON deals(user_id, status, stage_id, position);
CREATE INDEX deals_due ON deals(user_id, status, next_due);
CREATE INDEX deals_email ON deals(user_id, contact_email);
CREATE INDEX deals_company ON deals(user_id, company);
CREATE UNIQUE INDEX deals_client ON deals(user_id, client_key) WHERE client_key <> '';
CREATE UNIQUE INDEX deals_source ON deals(user_id, source_ref) WHERE source_ref <> '';

-- What happened on a deal: notes, calls, meetings, emails, steps done,
-- moves between stages, things made in the other apps.
CREATE TABLE activities (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  deal_id    INTEGER NOT NULL REFERENCES deals(id) ON DELETE CASCADE,
  kind       TEXT NOT NULL DEFAULT 'note',              -- note | call | meeting | email | step | stage | won | lost | made | created
  title      TEXT NOT NULL DEFAULT '',
  body       TEXT NOT NULL DEFAULT '',
  at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  app        TEXT NOT NULL DEFAULT '',                  -- an app it came from or went to
  url        TEXT NOT NULL DEFAULT '',
  source_app TEXT NOT NULL DEFAULT '',                  -- the app that wrote it through a Core link
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX activities_deal ON activities(deal_id, at);
CREATE INDEX activities_user ON activities(user_id, at);

-- What a deal has in the other Pocket apps (a project, an invoice, a
-- document to sign, a meeting, a task, a note…), made from here or found,
-- with a link to it — or why making it failed.
CREATE TABLE made (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  deal_id    INTEGER NOT NULL REFERENCES deals(id) ON DELETE CASCADE,
  target     TEXT NOT NULL,                             -- contact | task | event | note | email | signing | project | invoice
  app        TEXT NOT NULL DEFAULT '',
  ref        TEXT NOT NULL DEFAULT '',
  title      TEXT NOT NULL DEFAULT '',
  url        TEXT NOT NULL DEFAULT '',
  state      TEXT NOT NULL DEFAULT '',                  -- last seen there: sent, signed, paid…
  error      TEXT NOT NULL DEFAULT '',
  checked_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX made_user ON made(user_id, deal_id);
