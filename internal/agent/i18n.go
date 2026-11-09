package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Student-side texts. More languages can be added without recompiling by
// placing  agent-<code>.json  (same keys) next to OrbitAgent.exe.
var agentTexts = map[string]map[string]string{
	"en": {
		"ok":                "OK",
		"cancel":            "Cancel",
		"send":              "Send",
		"product":           "Orbit Classroom",
		"msg.title":         "Message from your teacher",
		"poll.title":        "Quick poll",
		"lock.title":        "Screen locked",
		"lock.text":         "Your teacher has locked this computer. Please pay attention.",
		"lock.input":        "Your mouse and keyboard are locked by the teacher.",
		"demo.title":        "Teacher's screen – Orbit",
		"demo.waiting":      "Waiting for the teacher's screen…",
		"tray.raise":        "Raise hand  (Ctrl+Alt+H)",
		"tray.lower":        "Lower hand  (Ctrl+Alt+H)",
		"tray.message":      "Message to teacher…",
		"tray.about":        "About Orbit Agent",
		"tray.exit":         "Exit",
		"tray.tip":          "Orbit Agent",
		"tray.tipLive":      "Orbit Agent – teacher is viewing",
		"tray.tipHand":      "Orbit Agent – hand raised",
		"hand.raised":       "Hand raised – your teacher has been notified.",
		"hand.lowered":      "Hand lowered.",
		"hand.byTeacher":    "Your teacher has seen your raised hand.",
		"msg.toTeacher":     "Message to your teacher",
		"msg.toTeacherBody": "Write a short message. Your teacher will see it in Orbit Master.",
		"msg.sent":          "Message sent to your teacher.",
		"notify.viewed":     "Your teacher is viewing your screen.",
		"notify.controlled": "Your teacher is controlling your computer.",
		"notify.shared":     "Your screen is being shown to the class.",
		"file.received":     "File received: %s",
		"focus.on":          "Focus mode is on – some apps are blocked during class.",
		"focus.off":         "Focus mode is off.",
		"focus.closed":      "Focus mode closed %s.",
		"about.title":       "Orbit Agent %s",
		"about.body":        "This computer is part of an Orbit Classroom.\n\nYour teacher can see small previews of your screen and may view or control it during class. You are always notified when that happens and the tray icon gets a red dot.\n\nRaise your hand: Ctrl+Alt+H\nStatus: %s",
		"status.none":       "no teacher connected",
		"status.some":       "connected to %d teacher session(s)",
		"setup.title":       "Set up Orbit Agent",
		"setup.body":        "This program lets your teacher's Orbit Master see and manage this computer.\n\nPaste the classroom key from Orbit Master (Settings → Teacher key). It starts with OPK1.",
		"setup.bodyFound":   "A classroom key file was found next to this program.\nKey fingerprint:  %s\n\nInstall Orbit Agent so it starts automatically for every user of this computer?",
		"setup.install":     "Install (recommended)",
		"setup.runOnce":     "Run once",
		"setup.badKey":      "That key is not valid: %s",
		"setup.done":        "Orbit Agent is installed and running. It starts automatically at every sign-in.",
		"setup.failed":      "Installation failed: %s",
		"setup.uninstalled": "Orbit Agent has been removed from this computer.",
		"setup.needAdmin":   "Installing needs administrator rights. Please confirm the Windows prompt.",
		"already":           "Orbit Agent is already running on this computer.",
		"portable.running":  "Orbit Agent is running (not installed). You find it in the system tray.",
		"error.port":        "Could not start: port %d is already in use. Is another Orbit Agent running?",
	},
	"sl": {
		"ok":                "V redu",
		"cancel":            "Prekliči",
		"send":              "Pošlji",
		"product":           "Orbit Classroom",
		"msg.title":         "Sporočilo učitelja",
		"poll.title":        "Hitra anketa",
		"lock.title":        "Zaslon je zaklenjen",
		"lock.text":         "Učitelj je zaklenil ta računalnik. Prosimo, bodite pozorni.",
		"lock.input":        "Učitelj je zaklenil vašo miško in tipkovnico.",
		"demo.title":        "Učiteljev zaslon – Orbit",
		"demo.waiting":      "Čakam na učiteljev zaslon …",
		"tray.raise":        "Dvigni roko  (Ctrl+Alt+H)",
		"tray.lower":        "Spusti roko  (Ctrl+Alt+H)",
		"tray.message":      "Sporočilo učitelju …",
		"tray.about":        "O programu Orbit Agent",
		"tray.exit":         "Izhod",
		"tray.tip":          "Orbit Agent",
		"tray.tipLive":      "Orbit Agent – učitelj gleda",
		"tray.tipHand":      "Orbit Agent – roka je dvignjena",
		"hand.raised":       "Roka je dvignjena – učitelj je obveščen.",
		"hand.lowered":      "Roka je spuščena.",
		"hand.byTeacher":    "Učitelj je videl vašo dvignjeno roko.",
		"msg.toTeacher":     "Sporočilo učitelju",
		"msg.toTeacherBody": "Napišite kratko sporočilo. Učitelj ga bo videl v programu Orbit Master.",
		"msg.sent":          "Sporočilo je poslano učitelju.",
		"notify.viewed":     "Učitelj si ogleduje vaš zaslon.",
		"notify.controlled": "Učitelj upravlja vaš računalnik.",
		"notify.shared":     "Vaš zaslon je prikazan celemu razredu.",
		"file.received":     "Prejeta datoteka: %s",
		"focus.on":          "Način zbranosti je vklopljen – nekateri programi so med poukom blokirani.",
		"focus.off":         "Način zbranosti je izklopljen.",
		"focus.closed":      "Način zbranosti je zaprl %s.",
		"about.title":       "Orbit Agent %s",
		"about.body":        "Ta računalnik je del učilnice Orbit Classroom.\n\nUčitelj vidi pomanjšan predogled vašega zaslona in ga lahko med poukom pogleda ali upravlja. O tem ste vedno obveščeni, ikona v sistemski vrstici pa dobi rdečo piko.\n\nDvig roke: Ctrl+Alt+H\nStanje: %s",
		"status.none":       "ni povezanega učitelja",
		"status.some":       "povezanih sej učitelja: %d",
		"setup.title":       "Nastavitev programa Orbit Agent",
		"setup.body":        "Ta program omogoča, da učiteljev Orbit Master vidi in upravlja ta računalnik.\n\nPrilepite ključ učilnice iz programa Orbit Master (Nastavitve → Učiteljev ključ). Začne se z OPK1.",
		"setup.bodyFound":   "Ob tem programu je datoteka s ključem učilnice.\nPrstni odtis ključa:  %s\n\nŽelite namestiti Orbit Agent, da se samodejno zažene za vse uporabnike tega računalnika?",
		"setup.install":     "Namesti (priporočeno)",
		"setup.runOnce":     "Zaženi enkrat",
		"setup.badKey":      "Ključ ni veljaven: %s",
		"setup.done":        "Orbit Agent je nameščen in deluje. Samodejno se zažene ob vsaki prijavi.",
		"setup.failed":      "Namestitev ni uspela: %s",
		"setup.uninstalled": "Orbit Agent je odstranjen s tega računalnika.",
		"setup.needAdmin":   "Za namestitev so potrebne skrbniške pravice. Potrdite okno sistema Windows.",
		"already":           "Orbit Agent že deluje na tem računalniku.",
		"portable.running":  "Orbit Agent deluje (ni nameščen). Najdete ga v sistemski vrstici.",
		"error.port":        "Zagon ni uspel: vrata %d so že zasedena. Ali že deluje drug Orbit Agent?",
	},
}

// Tr translates agent texts.
type Tr struct {
	lang string
	m    map[string]string
}

// NewTr loads the language (falls back to English for missing keys).
func NewTr(lang, exeDir string) *Tr {
	t := &Tr{lang: lang, m: map[string]string{}}
	for k, v := range agentTexts["en"] {
		t.m[k] = v
	}
	if m, ok := agentTexts[lang]; ok {
		for k, v := range m {
			t.m[k] = v
		}
	}
	if b, err := os.ReadFile(filepath.Join(exeDir, "agent-"+lang+".json")); err == nil {
		var ext map[string]string
		if json.Unmarshal(b, &ext) == nil {
			for k, v := range ext {
				t.m[k] = v
			}
		}
	}
	return t
}

// T returns the translated text, formatted with args when given.
func (t *Tr) T(key string, args ...any) string {
	s, ok := t.m[key]
	if !ok {
		s = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}
