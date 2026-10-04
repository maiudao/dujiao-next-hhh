import assert from 'node:assert/strict'
import test from 'node:test'
import { loadAllPages } from '../src/utils/loadAllPages.ts'

test('loadAllPages requests every reported page in order', async () => {
    const calls: number[] = []
    const items = await loadAllPages(async (page, pageSize) => {
        calls.push(page)
        assert.equal(pageSize, 200)
        return {
            items: [`product-${page}`],
            totalPages: 3,
        }
    })

    assert.deepEqual(calls, [1, 2, 3])
    assert.deepEqual(items, ['product-1', 'product-2', 'product-3'])
})

test('loadAllPages handles a single page when pagination is unavailable', async () => {
    const items = await loadAllPages(async () => ({ items: ['product-1'], totalPages: 0 }))

    assert.deepEqual(items, ['product-1'])
})
