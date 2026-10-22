// Restore a feature here when the shop is ready to offer it. Existing data is retained.
export const personalFeatures: Record<string, boolean> = {
  wallet: false,
  affiliate: false,
  reseller: false,
  giftCard: false,
  api: false,
  externalIdentity: false,
}

export function personalSectionEnabled(section: string): boolean {
  return personalFeatures[section] !== false
}

export function personalRouteEnabled(path: string): boolean {
  const routes: Record<string, string> = {
    '/me/wallet': 'wallet',
    '/me/affiliate': 'affiliate', '/me/reseller': 'reseller',
    '/me/gift-cards': 'giftCard', '/me/api': 'api', '/reseller': 'reseller',
  }
  return Object.entries(routes).every(([prefix, feature]) =>
    (path !== prefix && !path.startsWith(prefix + '/')) || personalSectionEnabled(feature))
}
