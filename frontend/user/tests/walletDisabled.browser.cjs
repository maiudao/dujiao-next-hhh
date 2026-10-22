// After pnpm build, run with a local Playwright module and Chromium executable.
// All responses are local fixtures, including intercepted payment submissions.
const { chromium } = require(process.env.SHOP_PLAYWRIGHT_MODULE || 'playwright')
const fs = require('node:fs'), path = require('node:path'), assert = require('node:assert/strict')
const dist = path.resolve(__dirname, '../dist'), origin = 'http://wallet-check.test'
const user = { id: 1, email: 'fixture@example.test', nickname: 'Local fixture', status: 'active' }
const channel = { id: 1, name: 'Local online payment', provider_type: 'official', channel_type: 'alipay', interaction_mode: 'qr', is_active: true }
const sku = { id: 1, sku_code: 'DEFAULT', price_amount: '50.00', manual_stock_total: -1, is_active: true }
const product = { id: 1, slug: 'fixture', title: { 'zh-CN': 'Local fixture product' }, price_amount: '50.00', fulfillment_type: 'manual', purchase_type: 'direct', manual_stock_total: -1, skus: [sku] }
const cart = { productId: 1, skuId: 1, skuCode: 'DEFAULT', slug: 'fixture', title: product.title, priceAmount: '50.00', quantity: 1, fulfillmentType: 'manual', purchaseType: 'direct', skuManualStockTotal: -1, skuStockEnforced: false }
const order = { id: 1, order_no: 'QA-WALLET-OFF', user_id: 1, status: 'pending_payment', currency: 'CNY', original_amount: '50.00', total_amount: '50.00', wallet_paid_amount: '0', online_paid_amount: '50.00', expires_at: new Date(Date.now() + 3600000).toISOString(), items: [{ id: 1, product_id: 1, sku_id: 1, title: product.title, quantity: 1, price_amount: '50.00', total_amount: '50.00', fulfillment_type: 'manual' }] }
async function main() {
  const browser = await chromium.launch({ executablePath: process.env.SHOP_CHROME_PATH || undefined, headless: true, args: ['--disable-gpu'] })
  const report = []
  try {
    for (const template of ['classic', 'vault']) for (const width of [1440, 390]) {
      const context = await browser.newContext({ viewport: { width, height: 950 }, locale: 'zh-CN', serviceWorkers: 'block' })
      const errors = [], walletRequests = [], paymentRequests = []
      await context.addInitScript(({ user, cart, template }) => {
        localStorage.setItem('user_token', 'local-fixture-token')
        localStorage.setItem('user_profile', JSON.stringify(user))
        localStorage.setItem('cart_items', JSON.stringify([cart]))
        localStorage.setItem('dj-storefront-template', template)
      }, { user, cart, template })
      await context.route('**/*', async route => {
        const req = route.request(), url = new URL(req.url())
        if (url.origin !== origin) return route.abort()
        if (url.pathname.startsWith('/api/')) {
          const p = url.pathname.replace('/api/v1', '')
          if (p.includes('/wallet')) walletRequests.push(p)
          let data = [], status_code = 0
          if (p === '/public/config') data = { brand: { name: 'Local Shop' }, languages: ['zh-CN'], currency: 'CNY', storefront_template: template, storefront_mode: 'normal', wallet_only_payment: true, payment_channels: [channel], captcha: { provider: 'none' } }
          else if (p === '/me') data = user
          else if (p === '/orders/preview') data = { original_amount: '50.00', total_amount: '50.00', currency: 'CNY', items: order.items }
          else if (p === '/order/payment-channels') data = [channel]
          else if (p === '/orders/QA-WALLET-OFF') data = order
          else if (p === '/payments/latest') data = null
          else if (/\/products\/(fixture|1)$/.test(p)) data = product
          else if (p.endsWith('/stats')) data = { by_status: {} }
          else if (['/payments', '/orders/create-and-pay'].includes(p) && req.method() === 'POST') {
            paymentRequests.push({ path: p, body: req.postDataJSON() }); status_code = 400; data = null
          }
          return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ status_code, msg: status_code ? 'Local intercepted payment' : 'success', data, pagination: { page: 1, total: 0, total_page: 1, page_size: 20 } }) })
        }
        let file = path.resolve(dist, '.' + decodeURIComponent(url.pathname))
        if (!file.startsWith(dist + path.sep) || !fs.existsSync(file) || !fs.statSync(file).isFile()) file = path.join(dist, 'index.html')
        const contentType = { '.html': 'text/html', '.js': 'application/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.woff2': 'font/woff2' }[path.extname(file)] || 'application/octet-stream'
        return route.fulfill({ status: 200, contentType, body: fs.readFileSync(file) })
      })
      const page = await context.newPage()
      page.on('pageerror', error => errors.push(error.message))
      await page.goto(origin + '/me')
      await page.getByText('Local fixture', { exact: true }).first().waitFor()
      assert.equal(await page.getByText('我的钱包', { exact: true }).count(), 0)
      for (const oldPath of ['/me/wallet', '/me/wallet/recharges/WR-OLD']) {
        await page.goto(origin + oldPath); await page.waitForURL(url => url.pathname === '/me')
      }
      await page.goto(origin + '/me/orders')
      await page.waitForFunction(() => document.body.innerText.includes('订单'))
      assert.equal(await page.getByRole('button', { name: '充值订单', exact: true }).count(), 0)
      for (const [location, endpoint] of [['/checkout', '/orders/create-and-pay'], ['/pay?order_no=QA-WALLET-OFF', '/payments']]) {
        await page.goto(origin + location)
        await page.getByRole('button', { name: /支付宝/ }).first().waitFor()
        assert.equal(await page.locator('.wallet-option').count(), 0)
        try { await page.getByRole('button', { name: /提交订单|立即支付|确认支付|发起支付/ }).last().click({ timeout: 10000 }) }
        catch (error) { throw new Error(location + ': ' + (await page.locator('body').innerText()).slice(-1300) + '\n' + error.message) }
        await page.waitForFunction(() => document.body.innerText.includes('Local intercepted payment'))
        const sent = paymentRequests.find(item => item.path === endpoint)
        assert.ok(sent, 'missing payment request ' + endpoint)
        assert.equal(sent.body.use_balance, false); assert.equal(sent.body.channel_id, 1)
      }
      assert.deepEqual(walletRequests, []); assert.deepEqual(errors, [])
      report.push({ template, width, oldLinksRedirect: true, walletHidden: true, walletRequests: 0, onlineSubmissions: paymentRequests.length })
      await context.close()
    }
    console.log(JSON.stringify({ pass: true, report, liveTraffic: false }))
  } finally { await browser.close() }
}
main().catch(error => { console.error(error); process.exitCode = 1 })
