import assert from 'node:assert/strict'
import test from 'node:test'
import { clampCharacterAnchor, clampCharacterPosition, differentIndex, readSupportState, safeSupportImage } from '../src/utils/supportCharacter.ts'
import { visibleStorefrontContacts } from '../src/utils/storefrontContacts.ts'
import { personalRouteEnabled, personalSectionEnabled } from '../src/utils/personalFeatures.ts'

test('character assets stay on the shop and invalid lists fall back', () => {
  for (const image of ['https://other.test/a.png', '//other.test/a.png', '/uploads/../a.png', '/uploads/%2e%2e/a.png', '/uploads/a.svg']) {
    assert.equal(safeSupportImage(image), '')
  }
  assert.equal(safeSupportImage(' /uploads/2026/a.webp '), '/uploads/2026/a.webp')
  assert.equal(readSupportState({ images: ['https://other.test/a.png'], messages: ['', null] }, 'open').images.length, 12)
})

test('each refresh can avoid the previous selection, including two assets', () => {
  for (let length = 2; length < 20; length++) {
    for (let previous = 0; previous < length; previous++) {
      for (const random of [0, .5, .999999]) {
        const next = differentIndex(length, previous, random)
        assert.notEqual(next, previous)
        assert.ok(next >= 0 && next < length)
      }
    }
  }
  assert.equal(differentIndex(1, 0, .8), 0)
  assert.equal(differentIndex(2, -1, .8), 1)
})

test('dragging keeps the whole character and bubble within the viewport', () => {
  assert.deepEqual(clampCharacterPosition(-100, -100, 190, 230, 390, 844), { x: 12, y: 82 })
  assert.deepEqual(clampCharacterPosition(1000, 1000, 190, 230, 390, 844), { x: 188, y: 594 })
})

test('character feet stay anchored when a fitting bubble opens or closes', () => {
  assert.deepEqual(clampCharacterAnchor(204, 728, 160, 92, 120, 390, 844), { x: 204, y: 728 })
  assert.deepEqual(clampCharacterAnchor(204, 728, 160, 92, 0, 390, 844), { x: 204, y: 728 })
  assert.deepEqual(clampCharacterAnchor(-30, 20, 160, 92, 120, 390, 844), { x: 12, y: 202 })
})

test('contacts show only enabled, nonempty entries and explicit configuration disables legacy fallback', () => {
  const rows = visibleStorefrontContacts({ qq: { enabled: false, value: 'private-number' }, wechat: { enabled: true, value: '' }, telegram: { enabled: true, value: 'https://t.me/example' } })
  assert.deepEqual(rows.map(row => row.key), ['telegram'])
  assert.equal(visibleStorefrontContacts({}, 'https://t.me/legacy').length, 0)
  assert.equal(visibleStorefrontContacts(undefined, 'https://t.me/legacy').length, 1)
})

test('paused account features hide their routes including nested reseller pages, while core pages remain usable', () => {
  for (const path of ['/me/affiliate', '/me/gift-cards', '/me/api', '/me/reseller', '/reseller', '/reseller/orders/123']) assert.equal(personalRouteEnabled(path), false)
  for (const path of ['/me', '/me/orders', '/me/wallet', '/me/security', '/me/profile', '/pay', '/products', '/me/apiary']) assert.equal(personalRouteEnabled(path), true)
  assert.equal(personalSectionEnabled('externalIdentity'), false)
})
