/* Shared display preference only. Authentication cookies must remain host-only.
 * Keep identical to dujiao-next/frontend/user/public/huihua-theme.js. */
(() => {
  'use strict';
  const cookieName = 'huihua-display-theme';
  const storageKeys = ['huihua-theme', 'dujiao_theme'];
  const sharedHosts = ['huihuahui.xyz', 'www.huihuahui.xyz', 'shop.huihuahui.xyz'];
  const isTheme = (value) => value === 'light' || value === 'dark';
  const system = window.matchMedia('(prefers-color-scheme: dark)');
  const readCookie = () => {
    try {
      const values = document.cookie.split(';').map((item) => item.trim());
      const entry = values.find((item) => item.startsWith(cookieName + '='));
      const value = entry ? entry.slice(cookieName.length + 1) : '';
      return isTheme(value) ? value : null;
    } catch { return null; }
  };
  const readLegacy = () => {
    for (const key of storageKeys) {
      try {
        const value = localStorage.getItem(key);
        if (isTheme(value)) return value;
      } catch {}
    }
    return null;
  };
  const saveLocal = (value) => {
    for (const key of storageKeys) {
      try { localStorage.setItem(key, value); } catch {}
    }
  };
  const saveCookie = (value) => {
    // Only the two Huihua sites share this non-sensitive preference.
    // Custom storefront domains keep a host-only preference.
    const domain = sharedHosts.includes(location.hostname) ? '; Domain=huihuahui.xyz' : '';
    const secure = location.protocol === 'https:' ? '; Secure' : '';
    try {
      document.cookie = cookieName + '=' + value + '; Max-Age=31536000; Path=/; SameSite=Lax' + domain + secure;
    } catch {}
  };
  let preference = readCookie() || readLegacy();
  let theme;
  const render = () => {
    const next = preference || (system.matches ? 'dark' : 'light');
    const root = document.documentElement;
    root.dataset.theme = next;
    root.classList.toggle('dark', next === 'dark');
    root.style.colorScheme = next;
    if (theme !== next) {
      theme = next;
      window.dispatchEvent(new CustomEvent('huihua:theme-change', { detail: { theme: next } }));
    }
  };
  const setTheme = (value) => {
    if (!isTheme(value)) return;
    preference = value;
    saveLocal(value);
    saveCookie(value);
    render();
  };
  const sync = () => {
    const shared = readCookie();
    if (shared && shared !== preference) {
      preference = shared;
      saveLocal(shared);
      render();
    }
  };
  // Migrate an explicit old selection once, without freezing the system default.
  if (preference) {
    saveLocal(preference);
    if (!readCookie()) saveCookie(preference);
  }
  render();
  window.HuihuaDisplayTheme = { get theme() { return theme; }, setTheme, sync };
  window.addEventListener('focus', sync);
  window.addEventListener('pageshow', sync);
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') sync();
  });
  // Cookies are shared across subdomains; storage/BroadcastChannel events are not.
  // No network request, and background pages do no polling work.
  window.setInterval(() => {
    if (document.visibilityState === 'visible') sync();
  }, 1500);
  if (typeof system.addEventListener === 'function') system.addEventListener('change', render);
  else if (typeof system.addListener === 'function') system.addListener(render);
})();
