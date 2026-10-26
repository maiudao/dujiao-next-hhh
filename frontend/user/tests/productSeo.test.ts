import { test } from 'node:test'
import assert from 'node:assert/strict'
import { productSeo } from '../src/utils/productSeo.ts'

test('metadata distinguishes real public variants without inventing claims', () => {
  const a = productSeo('Plus', ['A'], '', '公开说明', '')
  const b = productSeo('Plus', ['B'], '', '公开说明', '')
  assert.notEqual(a.title, b.title)
  assert.notEqual(a.description, b.description)
  assert.equal(productSeo('Plus', [], '明确标题', '', '<p>公开正文</p>').description, '明确标题：公开正文')
  assert.equal(Array.from(productSeo('商品', [], '', '字'.repeat(200), '').description).length, 160)
})
