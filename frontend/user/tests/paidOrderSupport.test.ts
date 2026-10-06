import test from 'node:test'
import assert from 'node:assert/strict'
import { orderSupportPhase, readPaidOrderMessage } from '../src/utils/paidOrderSupport.ts'

test('order-specific help only appears for paid orders', () => {
  for (const status of ['pending_payment', 'canceled', 'expired', 'refunded', 'partially_refunded', '']) {
    assert.equal(orderSupportPhase({ status }), '')
  }
  for (const status of ['paid', 'fulfilling', 'partially_delivered']) assert.equal(orderSupportPhase({ status }), 'paid')
  for (const status of ['delivered', 'completed']) assert.equal(orderSupportPhase({ status }), 'delivered')
})

test('paid and delivered messages use separate defaults and accept plain text', () => {
  assert.match(readPaidOrderMessage(null), /订单已付款/)
  assert.match(readPaidOrderMessage(' ', 'delivered'), /订单已交付成功/)
  assert.equal(readPaidOrderMessage(' 自定义正文 ', 'delivered'), '自定义正文')
  assert.equal(Array.from(readPaidOrderMessage('字'.repeat(350))).length, 300)
})
