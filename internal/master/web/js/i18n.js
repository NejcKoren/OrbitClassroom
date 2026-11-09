// Tiny i18n engine. Language files live in /i18n/<code>.json (embedded) and
// can be extended with lang/<code>.json next to OrbitMaster.exe.
const I18N = { dict: {}, lang: 'en', plural: new Intl.PluralRules('en') };

async function loadLang(code) {
  try {
    const r = await fetch('/api/i18n/' + encodeURIComponent(code));
    if (!r.ok) throw new Error(r.status);
    I18N.dict = await r.json();
    I18N.lang = code;
  } catch (e) {
    console.warn('language load failed', code, e);
    if (code !== 'en') return loadLang('en');
  }
  try { I18N.plural = new Intl.PluralRules(I18N.lang); } catch { I18N.plural = new Intl.PluralRules('en'); }
  document.documentElement.lang = I18N.lang;
}

function t(key, vars) {
  let s = I18N.dict[key];
  if (s === undefined) s = key;
  if (vars) for (const k in vars) s = s.split('{' + k + '}').join(vars[k]);
  return s;
}

// tn picks the plural form: key.one / key.two / key.few / key.other
function tn(key, n, vars = {}) {
  const cat = I18N.plural.select(n);
  const k = I18N.dict[key + '.' + cat] !== undefined ? key + '.' + cat : key + '.other';
  return t(k, { n, ...vars });
}

function applyI18n(root = document) {
  root.querySelectorAll('[data-i18n]').forEach(el => { el.textContent = t(el.dataset.i18n); });
  root.querySelectorAll('[data-i18n-ph]').forEach(el => { el.placeholder = t(el.dataset.i18nPh); });
  root.querySelectorAll('[data-i18n-title]').forEach(el => { el.title = t(el.dataset.i18nTitle); });
}

function fmtTime(unix) {
  const d = new Date(unix * 1000);
  return d.toLocaleTimeString(I18N.lang, { hour: '2-digit', minute: '2-digit' });
}

function fmtDateTime(unix) {
  const d = new Date(unix * 1000);
  return d.toLocaleString(I18N.lang, { dateStyle: 'medium', timeStyle: 'short' });
}

function fmtDuration(sec) {
  if (sec < 60) return t('time.sec', { n: sec });
  if (sec < 3600) return t('time.min', { n: Math.floor(sec / 60) });
  return t('time.hour', { n: Math.floor(sec / 3600), m: Math.floor((sec % 3600) / 60) });
}

function fmtBytes(b) {
  if (b < 1024) return b + ' B';
  if (b < 1048576) return (b / 1024).toFixed(1) + ' KB';
  if (b < 1073741824) return (b / 1048576).toFixed(1) + ' MB';
  return (b / 1073741824).toFixed(2) + ' GB';
}
