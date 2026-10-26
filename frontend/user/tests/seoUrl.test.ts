import test from 'node:test'
import assert from 'node:assert/strict'
import { absolutePublicUrl, normalizePublicOrigin, isPrivateStorefrontPath } from '../src/utils/seoUrl.ts'

test('legacy site configuration produces absolute HTTPS canonical and image URLs', () => {
  const origin = normalizePublicOrigin(' shop.huihuahui.xyz/ ')
  assert.equal(origin, 'https://shop.huihuahui.xyz')
  assert.equal(absolutePublicUrl('/products/gpt-plus-us', origin), 'https://shop.huihuahui.xyz/products/gpt-plus-us')
  assert.equal(absolutePublicUrl('/uploads/product.webp', origin), 'https://shop.huihuahui.xyz/uploads/product.webp')
  assert.equal(normalizePublicOrigin('http://localhost:5173'), 'http://localhost:5173')
  for (const value of ['https://example.com/path', 'https://user:secret@example.com', 'https://example.com?x=1', 'javascript:alert(1)']) assert.equal(normalizePublicOrigin(value), '')
  assert.equal(absolutePublicUrl('javascript:alert(1)', origin), '')
})

test('transaction and account routes stay outside public indexing', () => {
  for (const path of ['/me', '/me/orders', '/cart', '/auth/login', '/reseller/orders', '/checkout', '/pay', '/guest/order']) assert.equal(isPrivateStorefrontPath(path), true)
  for (const path of ['/', '/products/gpt-plus-us', '/about', '/blog/cart']) assert.equal(isPrivateStorefrontPath(path), false)
})
