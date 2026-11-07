// README screenshots and a browser smoke test, against a demo instance (DEMO_DATA=true seeds months of deals):
//
//   DATA_DIR=/tmp/crm-demo DEMO_DATA=true ADMIN_EMAIL=demo@example.com ADMIN_PASSWORD=demodemo1 ADMIN_NAME=Aleksandar PORT=18249 ./pocket-crm
//   CDP=http://127.0.0.1:9340 BASE=http://127.0.0.1:18249 node scripts/screenshots.mjs   # any Chrome with remote debugging
//   SHOTS=0 … node scripts/screenshots.mjs                                                # checks only
// playwright-core comes from web/node_modules, or set PW_FROM to a package.json next to an install of it.
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'

const require = createRequire(process.env.PW_FROM || new URL('../web/package.json', import.meta.url))
let pw
try { pw = require('playwright') } catch { pw = require('playwright-core') }
const { chromium } = pw
const base = process.env.BASE || 'http://127.0.0.1:18249'
const shots = process.env.SHOTS !== '0'
const only = process.env.ONLY || ''
const dir = process.env.OUT || new URL('../docs/screenshots/', import.meta.url).pathname
const browser = process.env.CDP ? await chromium.connectOverCDP(process.env.CDP) : await chromium.launch()
const errors = []

async function session({ phone = false, dark = false } = {}) {
  const ctx = await browser.newContext({
    viewport: phone ? { width: 390, height: 844 } : { width: 1440, height: 900 },
    deviceScaleFactor: phone ? 3 : 2, isMobile: phone, hasTouch: phone, colorScheme: dark ? 'dark' : 'light',
    userAgent: phone ? 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1' : undefined,
  })
  const page = await ctx.newPage()
  page.on('pageerror', e => errors.push(`pageerror: ${e}`))
  page.on('console', m => { if (m.type() === 'error' && !/Failed to load resource|Content Security Policy/.test(m.text())) errors.push(m.text()) })
  await page.goto(base + '/login')
  const res = await page.evaluate(() => fetch('/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: 'demo@example.com', password: 'demodemo1' }) }).then(r => r.status))
  assert.equal(res, 200, 'demo sign-in')
  return { ctx, page }
}
async function go(page, path, wait = 1500) {
  await page.goto(base + path, { waitUntil: 'networkidle' })
  // headless Chrome freezes CSS fade-ins: show everything at once
  await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' }).catch(() => {})
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(wait)
}
async function shot(page, name) { if (shots && (!only || only.split(',').includes(name))) await page.screenshot({ path: dir + name + '.png' }) }
const api = async (page, path) => page.evaluate(p => fetch('/api/v1' + p).then(r => r.json()), path)
const dealID = async (page, q, status = 'open') => (await api(page, `/deals?status=${status}&sort=value&q=` + encodeURIComponent(q))).items[0].id

// ---- desktop ----
{
  const { ctx, page } = await session()
  await go(page, '/', 1500)
  assert.ok((await page.content()).includes('Open deals'), 'Today shows the totals')
  await shot(page, 'desktop-today')
  await go(page, '/pipeline', 1500)
  assert.ok((await page.content()).includes('Proposal sent'), 'the board has its stages')
  await shot(page, 'desktop-pipeline')
  const id = await dealID(page, '')
  await go(page, `/deals/${id}`, 2000)
  assert.ok((await page.content()).includes('Timeline'), 'a deal shows its timeline')
  await shot(page, 'desktop-deal')
  await page.getByRole('button', { name: /^Won$/ }).first().click()
  await page.waitForTimeout(600)
  await shot(page, 'desktop-won')
  await page.keyboard.press('Escape')
  await go(page, '/deals', 1200)
  await shot(page, 'desktop-deals')
  await go(page, '/people', 1200)
  await shot(page, 'desktop-people')
  await go(page, '/reports', 1500)
  await shot(page, 'desktop-reports')
  await go(page, '/leads', 1000)
  await shot(page, 'desktop-leads')
  await go(page, '/settings/pipeline', 1200)
  await shot(page, 'desktop-settings-pipeline')
  await go(page, '/settings/apps', 1500)
  await shot(page, 'desktop-settings-apps')
  await go(page, '/pipeline', 1000)
  await page.keyboard.press('n')
  await page.waitForTimeout(600)
  await shot(page, 'desktop-new')
  await ctx.close()
}

// ---- phone ----
{
  const { ctx, page } = await session({ phone: true })
  await go(page, '/', 1500)
  await shot(page, 'phone-today')
  await go(page, '/pipeline', 1500)
  await shot(page, 'phone-pipeline')
  const id = await dealID(page, '')
  await go(page, `/deals/${id}`, 2000)
  await shot(page, 'phone-deal')
  await go(page, '/reports', 1500)
  await shot(page, 'phone-reports')
  await ctx.close()
}

// ---- dark ----
{
  const { ctx, page } = await session({ dark: true })
  await go(page, '/pipeline', 1500)
  await shot(page, 'desktop-pipeline-dark')
  await ctx.close()
}

await browser.close()
if (errors.length) { console.error(errors.join('\n')); process.exit(1) }
console.log('ok')
