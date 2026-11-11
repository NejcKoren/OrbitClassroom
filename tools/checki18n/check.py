# Checks that every UI text key exists in every language file (incl. plural forms).
import re, json, sys, glob, os
root = os.path.join(os.path.dirname(__file__), '..', '..', 'internal', 'master', 'web')
src = open(os.path.join(root, 'js', 'app.js'), encoding='utf-8').read() + open(os.path.join(root, 'js', 'i18n.js'), encoding='utf-8').read()
keys = set(re.findall(r"\bt\('([a-zA-Z0-9_.]+)'", src))
plurals = set(re.findall(r"\btn\('([a-zA-Z0-9_.]+)'", src))
keys |= set(re.findall(r"'((?:discover|state|tpl|poll)\.[a-zA-Z0-9_]+)'", src))
dyn = {
  'act.': [x + y for x in ['demo','lock','view','control','message','poll','website','run','file','screenshot','focus','power','more'] for y in ['', '.tip']],
  'filter.': ['all','online','offline','hands','locked'], 'theme.': ['dark','light','auto'], 'custom.type.': ['run','url','message'],
  'theme.name.': ['dark','slate','nord','obsidian','dusk','forest','ocean','light','paper','frost','contrast'],
  'poll.pre.': ['yesno','abcd','scale','understand','truefalse','speed'], 'focus.pre.': ['browsers','games','chat','media'],
  'power.': ['reboot','shutdown','logoff'], 'power.warnText.': ['reboot','shutdown'],
  'nav.': ['computers','discover','inbox','polls','shots','help','settings'],
  'done.': ['lock','unlock','message','open_url','run','reboot','shutdown','logoff','abort','hand_lower','focusOn','focusOff','poll'],
}
for p, items in dyn.items():
    keys.discard(p)
    for i in items: keys.add(p + i)
plurals.discard('power.confirm.')
for m in ['reboot','shutdown','logoff']: plurals.add('power.confirm.' + m)
forms = {'en': ['one','other'], 'sl': ['one','two','few','other']}
bad = 0
for f in sorted(glob.glob(os.path.join(root, 'i18n', '*.json'))):
    lang = os.path.basename(f)[:-5]
    d = json.load(open(f, encoding='utf-8'))
    missing = [k for k in sorted(keys) if k not in d and not any(k + '.' + x in d for x in ['one','other'])]
    pm = [k + '.' + x for k in sorted(plurals) for x in forms.get(lang, ['other']) if k + '.' + x not in d]
    for m in missing + pm: print(f'{lang}: missing {m}'); bad += 1
    print(f'{lang}: {len(d)} entries checked')
sys.exit(1 if bad else 0)
