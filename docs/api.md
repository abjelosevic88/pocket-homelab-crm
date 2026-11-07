# Pocket CRM API

Everything is under `/api/v1`, JSON in and out. Authenticate with a session cookie (`pcr_session`), a personal API token
(`Authorization: Bearer pcr_…`, Settings → API & shortcuts), or — for other Pocket apps — a Pocket Core link token
(`pcx_…`, see "Other apps" below). Every row belongs to the person who is signed in; other people's ids answer 404.

Money is kept in **cents**: `value` is `450000` for 4,500.00. Every deal also carries `amount` (`4500`) and its
`currency` (three letters, `EUR`, `BAM`, …). When adding a deal you may send either `value` (cents) or `amount`.

## Deals

| | |
|---|---|
| `GET /deals` | `?status=open\|won\|lost\|closed` `&stage=<id>` `&q=` `&company=` `&email=` `&due=overdue\|today\|week\|none\|any` `&starred=1` `&tag=` `&source=` `&sort=stage\|due\|value\|updated\|created\|close\|expected` `&limit=` `&offset=` → `{items: [Deal], total}` |
| `POST /deals` | a new deal (fields below; `stage` by name or `stage_id`, `note` = a first line for the timeline, `source_ref` = what it came from, never added twice) → 201 Deal (200 with the existing deal for a `source_ref` seen before) |
| `GET /deals/{id}` | `{deal, activities: [Activity], made: [Made], url}` |
| `PUT /deals/{id}` | change the fields sent (a `stage`/`stage_id` moves it) |
| `DELETE /deals/{id}` | |
| `POST /deals/{id}/move` | `{stage_id, before?, after?, lost_reason?, note?}` — `before`/`after` are the positions of the cards around it on the board |
| `POST /deals/{id}/step/done` | tick off the next step: `{kind?: step\|call\|meeting\|email, note?, next_action?, next_due?}` |
| `POST /deals/{id}/won` | win it and take it on: `{note?, project?, invoice?, task?, contact?, team_id?, name?, due?}` → `{deal, steps: [{target, made?, error?}]}` (each step on its own) |
| `POST /deals/{id}/make` | make one thing in another app: `{target: contact\|task\|event\|note\|email\|signing\|project\|invoice, …}` → Made |
| `GET /deals/{id}/elsewhere` | what the other apps know about the person, live: `[{app, name, state: ok\|off\|error\|none, detail, fix, items}]` |
| `GET /deals/{id}/meeting.ics` | `?date=&time=&minutes=&title=&location=&log=1` — a meeting as a calendar file (when Pocket Calendar is not connected) |
| `POST /deals/{id}/sync` | look now at what the deal has in the other apps (signed? paid? ticked off?) |
| `POST /deals/{id}/ai` | `{want: next\|summary\|email, hint?}` |
| `GET/POST /deals/{id}/activities` | the timeline; add `{kind: note\|call\|meeting\|email\|step, title, body, at?}` |
| `PUT/DELETE /activities/{id}` | lines a person wrote |
| `DELETE /made/{id}` | forget a linked item (it stays in the other app) |

**Deal**: `id, title, stage_id, stage_name, stage_color, status (open|won|lost), value, amount, currency, probability (-1 = the
stage's), chance, weighted, expected_close, source, contact_name, contact_email, contact_phone, contact_ref, company,
company_ref, description, next_action, next_due, next_task_ref, tags, starred, lost_reason, outcome_note, closed_at,
stage_changed_at, last_activity_at, source_app, source_ref, created_at, updated_at, overdue, stale, days_in_stage,
made_count, activities`. `contact_ref`/`company_ref` are card ids in Pocket Contacts.

## Pipeline, overviews

| | |
|---|---|
| `GET /stages` | `[{id, name, kind: open\|won\|lost, position, probability, color, deals, value}]` |
| `POST /stages`, `PUT /stages/{id}`, `DELETE /stages/{id}?move_to=`, `POST /stages/order {ids}` | |
| `GET /today` | `{lists: {overdue, today, week, no_step, stale, closing, won}, summary, date}` |
| `GET /summary` | `{currency, open, open_value, weighted, due_today, overdue, no_step, stale, won_month, won_month_value, won_year, won_year_value, lost_year, win_rate, closing_soon, url}` — `*_value` are `{currency: cents}` |
| `GET /people` | deals grouped by company (else person) `?q=` |
| `GET /lookup` | `?email=` and/or `?company=` → `{deals, url}` — for a contact's page in another app |
| `GET /search` | `?q=&limit=` → `{deals: [Deal + url], total}` |
| `GET /day/{YYYY-MM-DD}` | `{date, activities: [Activity + deal, deal_url], counts, due: [Deal], won, lost, created, url}` |
| `GET /reports` | `?months=12` → months, funnel, lost reasons, sources, days to win, average won |
| `GET /tags` | |
| `GET /leads`, `POST /leads/{answer id}/take` | answers to Pocket Forms not yet deals; take one as a deal |
| `GET /apps` | per linked app: `{slug, name, mode, via, state: on\|partly\|off\|link_off\|missing\|old\|no_core\|not_set_up\|error, detail, fix, on: [scopes], open_url}` |
| `GET /export.json`, `GET /export.csv`, `POST /import` | a Pocket CRM export (idempotent) or a CSV from Pipedrive, HubSpot, Zoho or a spreadsheet |

Web pages: `/` (Today), `/pipeline`, `/deals`, `/deals/{id}`, `/people`, `/leads`, `/reports`.

## Other apps (Pocket Core links)

CRM offers two scopes:

- `crm:read` — `GET /deals`, `/deals/{id}`, `/deals/{id}/activities`, `/stages`, `/summary`, `/today`, `/search`, `/people`,
  `/lookup`, `/reports`, `/tags`, `/day/{date}`
- `crm:write` — `POST /deals` (a lead; `source_ref` is kept as `<app>:<ref>` so the same thing is never added twice),
  `PUT /deals/{id}` (only deals that app added), `POST /deals/{id}/activities` (a line on any deal's timeline)

CRM uses: `contacts:read/write`, `mail:read`, `mail:send`, `calendar:read/write`, `tasks:read/write`, `notes:write`,
`signing:read/write`, `projects:read/write`, `invoices:read/write`, `forms:read`, and Pocket AI.

## Events

`crm.deal_created`, `crm.deal_moved`, `crm.deal_won`, `crm.deal_lost` — `{deal_id, title, stage, status, value, amount,
currency, contact_name, contact_email, company, next_action, next_due, url, user_email}` (+ `from_stage` when it moved);
`crm.steps_due` (each morning) — `{date, due, overdue, quiet, text, user_email}`.
