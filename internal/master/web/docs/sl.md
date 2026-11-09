# Orbit Classroom – navodila za uporabo

Različica 1.0 · slovenščina · Ta navodila so vgrajena tudi v Orbit Master (levi meni → **Pomoč in navodila**). Angleška različica je v datoteki GUIDE_EN.txt in v programu.

Orbit Classroom omogoča učitelju, da vidi in upravlja vse učenčeve računalnike v učilnici: sličice zaslonov v živo, predstavitev lastnega zaslona, zaklepanje zaslonov, oddaljeno upravljanje, sporočila, ankete, pošiljanje datotek, zagon programov in spletnih strani, posnetke zaslona, način zbranosti in upravljanje napajanja.

> Nasvet: vse, kar bi lahko bilo težje razumljivo, je razloženo s preprostimi besedami. Tehnične podrobnosti so označene z »za skrbnike IT«.

## 1. Hiter začetek (približno 10 minut)

1. Datoteko **OrbitMaster.exe** kopirajte na učiteljev računalnik in jo dvokliknite. Odpre se okno. (Windows lahko ob prvem zagonu prikaže modro opozorilo »Windows je zaščitil vaš računalnik« – kliknite **Več informacij → Vseeno zaženi**. Glejte 4. poglavje.)
2. V uvodnem ogledu izberite jezik.
3. Kliknite **Prenesi namestitveni paket** (v uvodnem ogledu, v razdelku *Poišči računalnike* ali v *Nastavitve → Učiteljev ključ*). Dobite datoteko **Orbit-Agent-Setup.zip**.
4. Datoteko zip kopirajte na vsak učenčev računalnik (USB ključ ali omrežna mapa), jo razširite in dvokliknite **Install-Orbit-Agent.cmd**. Potrdite skrbniško okno sistema Windows. To je vse na strani učenca – agent se odslej samodejno zažene za vsakega uporabnika.
5. Nazaj na učiteljevem računalniku: odprite **Poišči računalnike** in kliknite **Preišči omrežje**. Izberite računalnike in kliknite **Dodaj izbrane**.
6. Končano. Kliknite računalnik, da ga izberete, dvokliknite ga, da ga gledate v živo, ali uporabite gumbe v orodni vrstici za celo učilnico.

## 2. Dva programa

| Program | Kje | Kaj dela |
|---|---|---|
| **OrbitMaster.exe** | učiteljev računalnik | Konzola, s katero delate. Namestitev ni potrebna – samo zaženite ga. Vsebuje tudi program za učence, zato lahko sam izdela namestitveni paket. |
| **OrbitAgent.exe** | vsak učenčev računalnik | Teče nevidno v ozadju (majhna ikona Orbit ob uri). Učitelju pošilja slike zaslona in izvaja njegove ukaze. Namesti se enkrat. |

Datoteke v prenosu:

| Datoteka | Namen |
|---|---|
| OrbitMaster.exe | program za učitelja |
| OrbitAgent.exe | program za učence (vsebuje ga tudi OrbitMaster.exe) |
| GUIDE_EN.txt / NAVODILA_SL.txt | ta navodila v angleščini / slovenščini |
| README-FIRST.txt | hiter začetek na eni strani (angleško + slovensko) |
| lang\ | mapa za dodatne jezike (glejte 14.1) |

Zahteve: Windows 10 ali Windows 11 (64-bitni; priložena je tudi različica za ARM64). Za prikaz učiteljevega okna se uporablja Microsoft Edge (prednameščen v vsakem sistemu Windows 10/11). Vsi računalniki morajo biti v istem omrežju (glejte 6. poglavje).

## 3. Namestitev programa Orbit Agent na učenčeve računalnike

### 3.1 En računalnik naenkrat

1. Razširjeno mapo **Orbit Agent Setup** kopirajte na računalnik.
2. Dvokliknite **Install-Orbit-Agent.cmd** (ali OrbitAgent.exe).
3. Potrdite skrbniško okno (enkrat potrebujete skrbniško geslo).
4. Sporočilo potrdi namestitev. Ob uri se prikaže ikona Orbit.

Kaj naredi namestitveni program (za skrbnike IT):

- program kopira v `C:\Program Files\Orbit Classroom\OrbitAgent.exe`
- nastavitve zapiše v `C:\ProgramData\Orbit Classroom\agent.json` in tam ustvari potrdilo TLS; mapa je za učence *samo za branje* (Skrbniki/SYSTEM polni nadzor, Uporabniki branje)
- registrira samodejni zagon za vse uporabnike: `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Run\OrbitAgent`
- doda pravilo požarnega zidu Windows »Orbit Agent« (dohodne povezave za ta program)
- doda vnos v **Nastavitve → Aplikacije**, da ga lahko običajno odstranite

### 3.2 Več računalnikov hkrati (tiha namestitev)

Zaženite kot skrbnik ali kot SYSTEM (zagonska skripta pravilnika skupine, aplikacija Win32 v Intune, PDQ Deploy, SCCM, skripta …):

```
OrbitAgent.exe --install --silent
```

Ključ se prebere iz datoteke **orbit-agent.json** v isti mapi. Ključ lahko podate tudi neposredno (celotno vrstico kopirajte iz *Poišči računalnike → Namestitev na učenčeve računalnike*):

```
OrbitAgent.exe --install --silent --key OPK1-XXXX-XXXX-...
```

S `--silent` se ne prikaže nobeno okno. Agent se zažene ob naslednji prijavi (ali ponovno zaženite računalnik). Izhodna koda 0 = uspeh.

### 3.3 Preizkus brez namestitve

Dvokliknite OrbitAgent.exe in izberite **Zaženi enkrat**. Teče, dokler se uporabnik ne odjavi, v meniju ikone pa je ukaz **Izhod**. Primerno za preizkus; za šolsko uporabo ga vedno namestite.

### 3.4 Posodobitev na novo različico

Novi OrbitAgent.exe zaženite z `--install` (ali uporabite nov namestitveni paket) – zamenja stari program in ohrani nastavitve. Orbit Master in Orbit Agent iste glavne različice vedno delujeta skupaj.

### 3.5 Odstranitev

**Nastavitve → Aplikacije → Orbit Agent (Orbit Classroom) → Odstrani** ali `OrbitAgent.exe --uninstall` (za skripte dodajte `--silent`).

## 4. Varnostna opozorila sistema Windows – kaj pomenijo

**»Windows je zaščitil vaš računalnik« (SmartScreen).** Programi, ki niso digitalno podpisani s plačljivim potrdilom ali so novi, sprožijo to modro opozorilo. Kliknite **Več informacij → Vseeno zaženi**. Oddelek IT se mu lahko izogne tako, da datoteke .exe podpiše s šolskim potrdilom za podpisovanje kode ali jih dovoli v programu Windows Defender / Intune.

**Protivirusni program.** Programe, ki lahko gledajo in upravljajo zaslone, protivirusni programi včasih označijo, ker se orodja za oddaljeno upravljanje lahko zlorabijo. Če je OrbitAgent.exe blokiran, dodajte izjemo za `C:\Program Files\Orbit Classroom\`.

**Požarni zid.** Namestitveni program pravilo požarnega zidu ustvari samodejno. Če uporabljate drug požarni zid, za OrbitAgent.exe dovolite dohodna vrata TCP 21400 in UDP 21401.

**Skrbniško okno.** Skrbniške pravice potrebuje samo *namestitev* agenta. Orbit Master jih nikoli ne potrebuje.

## 5. Učiteljev ključ – preprosto razloženo

Ključ si predstavljajte kot **ključ in ključavnico**:

- **Skrivni učiteljev ključ** (začne se z `OTK1-`) je ključ. Ostane v programu Orbit Master na vašem računalniku.
- **Javni ključ** (začne se z `OPK1-`) je pripadajoča ključavnica. Vgrajen je v namestitveni paket in nameščen na vsak učenčev računalnik.
- Učenčev računalnik uboga samo Orbit Master, katerega ključ ustreza njegovi ključavnici. Učenci z javnim ključem ne morejo upravljati drugih računalnikov – z njim ne odprejo ničesar.

Tehnično (za skrbnike IT): podpisi Ed25519 z novim izzivom ob vsaki prijavi, vezani na potrdilo TLS agenta (zaščita pred ponovitvijo in posredovanjem »človek v sredini«); ves promet je šifriran s TLS.

Pomembne situacije:

| Situacija | Kaj storiti |
|---|---|
| Več učiteljev naj upravlja iste učilnice | Na prvem učiteljevem računalniku: *Nastavitve → Učiteljev ključ → Varnostna kopija skrivnega ključa*. Na drugih učiteljevih računalnikih: *Uvozi skrivni ključ*. |
| Nov učiteljev računalnik / ponovno nameščen Windows | Uvozite varnostno kopijo skrivnega ključa – učenčevih računalnikov ni treba spreminjati. |
| Skrivni ključ je izgubljen (ni varnostne kopije) | *Ustvari nov ključ*, nato na vse učenčeve računalnike namestite nov namestitveni paket (namestitveni program doda nov ključ). |
| Skrivni ključ je bil morda ukraden | *Ustvari nov ključ* in znova namestite agente; nato iz `agent.json` odstranite stari ključ (glejte 14.4). |
| Računalnik kaže »Ključ ni sprejet« | Nameščen je bil z drugim ključem. Nanj namestite svoj namestitveni paket. |

Prstni odtis (npr. `4E18-83B9-7FD0`) je kratko »ime« vašega ključa, da lahko ključe hitro primerjate.

## 6. Omrežne zahteve

| Vrata | Smer | Uporaba |
|---|---|---|
| TCP 21400 | učitelj → učenec | vse (šifrirano) |
| UDP 21401 | učitelj → učenec (oddajanje) | samodejno iskanje računalnikov |
| UDP 9 in 7 | učitelj → omrežje (oddajanje) | Wake-on-LAN (vklop) |
| TCP 21480 | samo znotraj učiteljevega računalnika (127.0.0.1) | učiteljevo okno in API za avtomatizacijo |

- Učiteljev računalnik vzpostavlja samo **odhodne** povezave – na njem ni treba odpirati ničesar.
- Učiteljev in učenčevi računalniki naj bodo v istem omrežju (ista stavba / VLAN). Samodejno iskanje uporablja oddajanje (broadcast), ki ne prehaja usmerjevalnikov. V drugih omrežjih računalnike dodajte z imenom ali naslovom IP ali uporabite *temeljito iskanje*.
- **Wake-on-LAN** (vklop) deluje samo, če je omogočen v nastavitvah BIOS/UEFI in omrežnega vmesnika, je računalnik povezan s kablom in je v istem omrežju. Naslov MAC se samodejno zapomni, ko je računalnik prvič povezan.
- Pasovna širina: sličice so majhne in se pošljejo samo, ko se slika spremeni. Ogled v živo in predstavitev pošiljata samo spremenjene dele zaslona; mirujoča prosojnica ne stane skoraj nič. Za 30 računalnikov običajno šolsko omrežje z 1 Gbit/s povsem zadostuje. V počasnih omrežjih znižajte hitrost sličic predstavitve (Nastavitve → Hitrost in kakovost).

## 7. Učilnice in računalniki

- **Učilnice** (levi meni) združujejo računalnike – npr. ena učilnica za vsako računalniško učilnico. Novo ustvarite z **+**. Preimenujete ali izbrišete jo z gumbom **⋯**, ki se prikaže, ko pokažete na učilnico. Brisanje učilnice jo odstrani samo iz programa Orbit Master.
- **Poišči računalnike** → **Preišči omrežje** prikaže vse delujoče programe Orbit Agent. Stolpec *Vaš ključ*: **zaupa** pomeni, da je bil nameščen z vašim ključem; **drug ključ** pomeni, da pripada drugemu učiteljevemu ključu. Novi računalniki so že izbrani – izberite učilnico in kliknite **Dodaj izbrane**.
- **Temeljito iskanje** preizkusi vse naslove vašega lokalnega omrežja – uporabite ga, če običajno iskanje ne najde ničesar (nekatera omrežja blokirajo oddajanje).
- **Ročno dodajanje**: vnesite ime računalnika (npr. `PC-12`) ali naslov IP.
- **Urejanje**: desni klik na računalnik → *Uredi računalnik* (ime, naslov, učilnica, naslov MAC).
- **Razvrščanje**: računalnik povlecite na drugega, da ga postavite pred njega – sličice razporedite kot sedeže v učilnici. **Premik v drugo učilnico**: sličice povlecite na učilnico v levem meniju.

Barvne pike stanja: zelena = povezan, rumena = povezovanje, siva = ni povezave, rdeča = ključ ni sprejet ali nezdružljiva različica.

## 8. Delo pri pouku – vse funkcije

### 8.1 Na katere računalnike vpliva dejanje?

Poglejte vrstico **Velja za** nad sličicami:

- Če ste računalnike **izbrali**, dejanja vplivajo samo nanje.
- Če ni izbran noben, dejanja vplivajo na **vse prikazane povezane računalnike** (upoštevajoč filtre in polje za filtriranje).

Izbiranje: klik = izberi enega · Ctrl+klik = dodaj/odstrani · Shift+klik = obseg · vlečenje po praznem prostoru = izbirni okvir · Ctrl+A = vsi · Esc = počisti.

**Filtri** (Vsi, Povezani, Nepovezani, Dvignjene roke, Zaklenjeni) in **polje za filtriranje** (ime, uporabnik ali program) omejijo prikaz. **Drsnik** spremeni velikost sličic.

### 8.2 Sličice in dejavnost

Vsaka sličica prikazuje zaslon v živo, ime računalnika, prijavljenega uporabnika, aktivni program in koliko časa je učenec nedejaven (po 5 minutah rumeno). Oznake: rdeča **V živo** (zaslon si nekdo prav zdaj ogleduje ali ga upravlja), ključavnica, predstavitev, zbranost in rumena mahajoča roka za dvignjene roke. Prikaz dejavnosti lahko izklopite v nastavitvah.

**Poudarek** (Več → Poudari izbrane ali tipka S): prikaže samo izbrane računalnike, velike in s hitrejšim osveževanjem.

### 8.3 Predstavitev

- **Predstavitev čez cel zaslon**: učenci vidijo samo vaš zaslon; njihova tipkovnica in miška sta blokirani.
- **Predstavitev v oknu**: vaš zaslon se prikaže v oknu – učenci lahko delajo naprej.
- **Deli zaslon učenca**: zaslon enega učenca pokaže vsem drugim računalnikom (odlično za prikaz dobre rešitve).
- Vrstica na vrhu prikazuje predstavitev v teku; **Ustavi** jo konča.

Prikazan je vaš glavni monitor. Učenci, ki se pridružijo pozneje, dobijo sliko v nekaj sekundah.

### 8.4 Zaklepanje

- **Zakleni zaslone** (tipka L): slika »Zaslon je zaklenjen« čez cel zaslon; tipkovnica in miška sta blokirani.
- **Zakleni samo miško in tipkovnico**: zaslon ostane viden, a ga ni mogoče uporabljati (npr. ko razlagate, kar je na njihovem zaslonu).
- **Zakleni z lastnim besedilom**: vaš naslov in besedilo. Privzeto besedilo lahko nastavite v nastavitvah.
- Za odklepanje znova kliknite gumb (zdaj se imenuje **Odkleni**). Zaklepanje ostane dejavno tudi, če Orbit Master zaprete.

Opomba: Windows vedno dovoli Ctrl+Alt+Del – tega ne more blokirati noben program. Učenci, ki ga pritisnejo, vidijo samo varnostni zaslon sistema Windows in se lahko vrnejo.

### 8.5 Ogled in upravljanje

Dvokliknite sličico (ali jo izberite in pritisnite V / C). Odpre se pregledovalnik:

- Stikalo **Ogled / Upravljanje**. V načinu *Upravljanje* vaša miška in tipkovnica delujeta na učenčevem računalniku.
- Gumbi za **Win**, **Alt+Tab**, **Ctrl+Shift+Esc** (tipke, ki bi jih sicer prestregel vaš Windows) in **Vnesi besedilo** (vtipka poljubno besedilo, tudi č, š, ž).
- Izbira **monitorja**, če ima učenec več zaslonov. **Kakovost**: samodejno/nizka/srednja/visoka. **Prilagodi** preklaplja med prilagoditvijo oknu in dejansko velikostjo.
- **Posnetek zaslona**, **Snemaj video** (shrani se kot .webm v mapo Prenosi), **Cel zaslon** (v celozaslonskem načinu se učencu pošljejo tudi tipke, kot je Ctrl+W).
- Zgoraj desno so prikazani ločljivost, število sličic na sekundo in pasovna širina.
- V načinu ogleda Esc zapre pregledovalnik; v načinu upravljanja uporabite gumb ✕ (Esc se pošlje učencu).

Učenec vidi obvestilo in rdečo piko na ikoni, dokler si nekdo ogleduje ali upravlja njegov zaslon.

Windows nekatere zaslone ščiti: **skrbniškega okna UAC**, **zaslona Ctrl+Alt+Del** in **zaklenjenega zaslona Windows** ni mogoče gledati ali upravljati (slika zamrzne, vnos se prezre). Prav tako ni mogoče upravljati programov, ki na učenčevem računalniku tečejo »kot skrbnik«. To je varnostna funkcija sistema Windows.

### 8.6 Sporočilo

Učencem pošlje okno s sporočilom (tipka M). Na voljo so pripravljene predloge. Neobvezni naslov zamenja besedilo »Sporočilo učitelja«.

### 8.7 Anketa

Postavite vprašanje z do 12 odgovori (tipka P). Pripravljeni odgovori: Da/Ne, A–D, 1–5, razumevanje, drži/ne drži, tempo. Učenci odgovorijo s klikom na gumb v pojavnem oknu. Stran **Ankete** prikazuje graf v živo, odgovor vsakega učenca in kdo še ni odgovoril. **Zaključi anketo** odstrani okno z učenčevih računalnikov; **CSV** izvozi rezultate (odprete jih v Excelu).

### 8.8 Spletna stran in Zaženi program

- **Spletna stran** odpre spletni naslov v učenčevem privzetem brskalniku (samo http:// in https://).
- **Zaženi** zažene program, dokument ali ukaz kot prijavljeni učenec – kot dvoklik (npr. `notepad.exe`, `winword.exe`, `C:\Apps\Geogebra\GeoGebra.exe` ali pot do dokumenta). Argumenti so neobvezni.
- Obe okni ponujata **priljubljene**; označite *Shrani med priljubljene* ali jih uredite v nastavitvah.

### 8.9 Pošlji datoteko

Datoteko spustite v okno. Kopira se na vsak izbran računalnik v mapo **Namizje\Orbit** (obstoječa datoteka se ne prepiše – doda se »(2)«). Datoteko je mogoče tudi samodejno odpreti.

### 8.10 Posnetek zaslona

Shrani slike PNG izbranih zaslonov v polni ločljivosti v mapo `Slike\Orbit Screenshots` (nastavljivo). Prikaže jih stran **Posnetki zaslona**; s klikom jih povečate.

### 8.11 Način zbranosti

Na učenčevih računalnikih samodejno zapira navedene programe (igre, klepetalnike, brskalnike …), dokler je način zbranosti vklopljen. Pripravljeni seznami dodajo pogoste programe. Učenci dobijo obvestilo. Programi se zaprejo **brez vprašanja**, zato se neshranjeno delo v njih izgubi. Izklopite ga v istem oknu.

### 8.12 Napajanje

- **Vklopi** (Wake-on-LAN) za nepovezane računalnike – zahteve so v 6. poglavju.
- **Znova zaženi**, **Zaustavi sistem**, **Odjavi uporabnika**. Z možnostjo *Učence opozori 60 sekund prej* Windows prikaže sporočilo z odštevanjem; **Prekliči čakajočo zaustavitev** ga ustavi.
- Vprašanje za potrditev lahko izklopite v nastavitvah.

### 8.13 Dvignjene roke in sporočila učencev

Učenci kliknejo ikono Orbit ob uri → **Dvigni roko** (ali pritisnejo **Ctrl+Alt+H**) ali **Sporočilo učitelju …**. Zaslišite zvok in vidite obvestilo; na sličici se prikaže mahajoča roka. Stran **Sporočila** prikazuje dvignjene roke (najstarejše najprej) in sporočila; lahko odgovorite, si ogledate zaslon ali spustite roko.

### 8.14 Ukazna paleta

Pritisnite **Ctrl+K** in začnite tipkati: katero koli dejanje, učilnico, računalnik, nastavitev ali jezik. S puščicami in tipko Enter ga zaženete. Najhitrejši način dela.

## 9. Bližnjice na tipkovnici

| Tipka | Dejanje |
|---|---|
| Ctrl+K | ukazna paleta |
| Ctrl+A | izberi vse prikazane računalnike |
| Esc | počisti izbor / zapri |
| L / U | zakleni / odkleni |
| M | sporočilo |
| P | anketa |
| V / C | ogled / upravljanje (en izbran računalnik) |
| D | meni predstavitve |
| S | poudarek vklop/izklop |
| + / − | večje / manjše sličice |
| Alt+1 … Alt+9 | preklop učilnice |
| ? | ta navodila |
| Ctrl+Enter | potrdi odprto okno |

Učenci: **Ctrl+Alt+H** dvigne ali spusti roko.

## 10. Kaj vidijo učenci (zasebnost in preglednost)

- Ikono Orbit ob uri. Njena pika pomeni: **rdeča** = učitelj si prav zdaj ogleduje ali upravlja zaslon, **rumena** = dvignjena roka, **siva** = zaklenjeno.
- Obvestilo sistema Windows, ko si učitelj začne ogledovati ali upravljati zaslon, ko je zaslon prikazan razredu in ko način zbranosti zapre program.
- *O programu Orbit Agent* v meniju ikone s preprostimi besedami razloži, kaj vidi učitelj.

Priporočila: v šolskih pravilih o uporabi IT obvestite učence in starše, da se šolski računalniki med poukom spremljajo in kaj se beleži. Orbit sam ničesar ne snema – posnetki zaslona in videoposnetki nastanejo samo, ko učitelj klikne gumb, in se shranijo samo na učiteljev računalnik. (Za skrbnike IT: nastavitev agenta `notify_on_view` lahko spremenijo samo skrbniki.)

## 11. Pregled nastavitev

| Nastavitev | Pomen |
|---|---|
| Jezik | jezik učiteljevega programa (programi za učence uporabljajo jezik sistema Windows) |
| Videz / Barva poudarka | temna, svetla ali sistemska tema; barva gumbov in poudarkov |
| Zvoki | zvok ob dvignjeni roki in sporočilu |
| Prikaži dejavnost učencev | aktivni program in čas nedejavnosti na sličicah |
| Potrdi dejanja napajanja | vprašaj pred ponovnim zagonom / zaustavitvijo / odjavo |
| Osveževanje sličic | kako pogosto se sličice posodobijo (privzeto 1 s) |
| Kakovost / hitrost sličic / največja širina oddaljenega ogleda | kakovost slike v primerjavi s pasovno širino pregledovalnika |
| Kakovost / hitrost sličic / največja širina predstavitve | enako za predstavitve (v počasnih omrežjih nižja hitrost sličic) |
| Zaklenjen zaslon | privzeti naslov in besedilo zaklenjenih zaslonov |
| Učiteljev ključ | javni ključ, namestitveni paket, varnostna kopija/uvoz/nov ključ (5. poglavje) |
| Priljubljene | spletne strani in programi za okna |
| Dejanja po meri | vaši lastni gumbi (14.2) |
| API za avtomatizacijo | žeton za skripte (14.3) |
| Datoteke in mape | mapa z nastavitvami, mapa za posnetke zaslona |

## 12. Odpravljanje težav

| Težava | Rešitev |
|---|---|
| »Windows je zaščitil vaš računalnik« | Več informacij → Vseeno zaženi (4. poglavje). |
| Iskanje ne najde ničesar | Je agent nameščen in je na učenčevem računalniku vidna ikona Orbit? Isto omrežje? Poskusite s *temeljitim iskanjem*. Preverite požarni zid/protivirusni program. Računalnik dodajte z naslovom IP. |
| Sličica kaže **Ni povezave** / »Orbit Agent ne deluje« | Računalnik je izklopljen, nihče še ni prijavljen (agent se zažene ob prijavi) ali je bil agent zaprt. Prijavite se ali ponovno zaženite računalnik. |
| Sličica kaže **Ključ ni sprejet** | Računalnik je bil nameščen z drugim učiteljevim ključem – namestite svoj namestitveni paket (5. poglavje). |
| Sličica kaže **Nezdružljiva različica** | Orbit Master in Orbit Agent posodobite na isto različico. |
| Ni odgovora / časovna omejitev | Požarni zid blokira TCP 21400 ali je računalnik v drugem omrežju. |
| Slika je črna ali zamrznjena | Na učenčevem računalniku je prikazano okno UAC ali zaklenjen zaslon Windows (8.5). |
| Upravljanje v enem programu ne deluje | Ta program teče kot skrbnik (8.5). |
| Vklop ne naredi ničesar | Wake-on-LAN je onemogočen v BIOS-u / omrežnem vmesniku, samo Wi-Fi ali drugo omrežje (6. poglavje). |
| Učiteljevo okno se ne odpre | Microsoft Edge manjka ali je blokiran – Orbit nato odpre vaš privzeti brskalnik. Naslov okna je zapisan tudi v dnevniški datoteki. |
| Dva učiteljeva računalnika, eden je zavrnjen | Na obeh uvozite isti skrivni ključ (5. poglavje). |
| Učenec je zaprl agenta | Računalnik je prikazan kot nepovezan. Ponovni zagon ali odjava/prijava ga znova zažene. |
| »Vrata 21400 so že zasedena« | Teče drug Orbit Agent (drug prijavljen uporabnik na istem računalniku). Podprta je samo ena seja na računalnik. |

Dnevniške datoteke: `%APPDATA%\Orbit Classroom\logs\master.log` (učitelj) in `%APPDATA%\Orbit Classroom\logs\agent.log` (učenec, za vsakega uporabnika). Pot vnesite v naslovno vrstico Raziskovalca.

## 13. Datoteke in mape

| Pot | Vsebina |
|---|---|
| `%APPDATA%\Orbit Classroom\master.json` | učiteljeve nastavitve, učilnice, računalniki **in skrivni ključ** – učiteljev račun Windows zaščitite z geslom |
| `%APPDATA%\Orbit Classroom\logs\` | dnevniške datoteke |
| `%APPDATA%\Orbit Classroom\ui-profile\` | zasebni profil brskalnika za učiteljevo okno (lahko ga izbrišete) |
| `Slike\Orbit Screenshots\` | posnetki zaslona |
| `C:\Program Files\Orbit Classroom\` | nameščen agent |
| `C:\ProgramData\Orbit Classroom\agent.json` | nastavitve agenta (za učence samo za branje) |
| `C:\ProgramData\Orbit Classroom\agent-cert.pem / agent-key.pem` | potrdilo TLS agenta |
| `Namizje\Orbit\` (učenec) | prejete datoteke |

## 14. Razširjanje programa Orbit Classroom (možnosti za prihodnost)

Orbit je zasnovan tako, da ga lahko v pogostih primerih izboljšate brez programiranja, za vse ostalo pa je na voljo celotna izvorna koda.

### 14.1 Dodajanje jezika

1. Kopirajte `en.json` iz izvorne mape `internal\master\web\i18n\` in prevedite besedila na desni strani. Oznak `{n}`, `{name}` ipd. ne spreminjajte. `"_name"` nastavite na ime jezika, npr. `"Deutsch"`.
2. Shranite ga kot `lang\de.json` ob datoteki OrbitMaster.exe (uporabite dvočrkovno kodo jezika). Znova zaženite Orbit Master – jezik se prikaže v nastavitvah.
3. Neobvezno: prevedena navodila shranite kot `lang\de.md` ob OrbitMaster.exe – prikažejo se v razdelku Pomoč.
4. Neobvezno za učence: ob OrbitAgent.exe (nameščen: `C:\Program Files\Orbit Classroom\`) ustvarite `agent-de.json` z besedili agenta (ključi so v `internal\agent\i18n.go`). Agent samodejno uporabi jezik prikaza sistema Windows.

Manjkajoča besedila se samodejno prikažejo v angleščini, zato delujejo tudi delni prevodi.

### 14.2 Dejanja po meri

Nastavitve → Dejanja po meri → **Dodaj dejanje**. Vrste: *Zaženi program* (program + argumenti), *Odpri spletno stran* (URL), *Pošlji sporočilo* (besedilo). Prikažejo se v meniju **Več** in v ukazni paleti (Ctrl+K). Primer: ime »GeoGebra«, vrsta Zaženi, vrednost `C:\Program Files\GeoGebra\GeoGebra.exe`.

### 14.3 API za avtomatizacijo (za skrbnike IT)

Orbit Master ponuja lokalni vmesnik REST na `http://127.0.0.1:21480/api` (dosegljiv samo z učiteljevega računalnika). V Nastavitve → API za avtomatizacijo ustvarite žeton in ga pošiljajte kot `Authorization: Bearer <žeton>`.

| Metoda in pot | Namen |
|---|---|
| GET /api/state | učilnice, računalniki s stanjem v živo, sporočila, anketa, predstavitev |
| POST /api/action | `{"targets":[id-ji],"action":"lock","params":{...}}` – dejanja: lock, unlock, message, open_url, run, power, focus, hand_lower, demo_stop |
| POST /api/wake | `{"targets":[id-ji]}` Wake-on-LAN |
| POST /api/demo/start · /api/demo/stop | `{"source":"teacher" ali id,"mode":"fullscreen" ali "window","targets":[id-ji]}` |
| POST /api/poll · /api/poll/close | `{"targets":[id-ji],"question":"…","options":["Da","Ne"]}` |
| POST /api/screenshot | `{"targets":[id-ji]}` |
| POST /api/file | obrazec multipart: targets (JSON), open (0/1), file |
| POST /api/computers · PUT/DELETE /api/computers/{id} | upravljanje računalnikov |
| POST /api/rooms · PUT/DELETE /api/rooms/{id} | upravljanje učilnic |
| POST /api/discover | `{"scan":false}` iskanje po omrežju |
| GET /api/thumb/{id} | trenutna sličica (JPEG) |

Parametri `params`: lock → mode ("screen"/"input"), title, text · message → title, text · open_url → url · run → program, args · power → mode ("reboot", "shutdown", "logoff", "abort"), delay (sekunde), text · focus → enabled, list (imena programov).

Primer za PowerShell (npr. zaklepanje vseh računalnikov ob koncu ure z Načrtovalnikom opravil):

```
$h = @{ Authorization = "Bearer orb_vas_zeton" }
$s = Invoke-RestMethod -Headers $h http://127.0.0.1:21480/api/state
$ids = $s.computers | Where-Object { $_.state -eq "online" } | ForEach-Object { $_.id }
$body = @{ targets = $ids; action = "lock" } | ConvertTo-Json
Invoke-RestMethod -Method Post -Headers $h -ContentType "application/json" -Body $body http://127.0.0.1:21480/api/action
```

Orbit Master mora delovati. Za samodejno delovanje brez okna ga zaženite z `OrbitMaster.exe --no-window`.

### 14.4 Možnosti agenta (agent.json, za skrbnike IT)

Kot skrbnik uredite `C:\ProgramData\Orbit Classroom\agent.json` (ali pred namestitvijo uredite `orbit-agent.json` v namestitvenem paketu). Spremembe veljajo po naslednji prijavi.

| Možnost | Privzeto | Pomen |
|---|---|---|
| trusted_keys | vaš ključ | seznam javnih učiteljevih ključev (OPK1-…), ki lahko upravljajo ta računalnik |
| port | 21400 | vrata TCP |
| discovery | true | odgovarjaj na samodejno iskanje po omrežju |
| language | auto | jezik za učence: auto, en, sl, … |
| notify_on_view | true | obvesti učence, ko si nekdo ogleduje/upravlja zaslon |
| allow_control | true | dovoli oddaljeno miško in tipkovnico (false = samo ogled) |
| show_tray | true | prikaži ikono Orbit ob uri |
| allow_hand | true | funkcija dviga roke |
| allow_messages | true | sporočila učenec → učitelj |
| share_activity | true | sporočaj aktivni program in čas nedejavnosti |
| received_folder | (Namizje\Orbit) | mapa za prejete datoteke |

### 14.5 Prevajanje iz izvorne kode (za razvijalce)

Celotna izvorna koda je v OrbitClassroom-1.0.0-source.zip. Napisana je v jeziku Go (drugih orodij ali knjižnic ni treba):

```
build.cmd          (v sistemu Windows: potreben je Go s https://go.dev/dl/)
./build.sh         (v Linuxu/macOS: izdela pakete x64 + ARM64 z ikonami)
```

Zgradba projekta:

| Mapa | Vsebina |
|---|---|
| cmd/master, cmd/agent | vstopni točki programov |
| internal/proto | skupni protokol: ključi, prijava, oblika toka zaslona, iskanje |
| internal/agent | agent za učence: strežnik API, ukazi, namestitev, besedila |
| internal/master | učiteljeva konzola: povezave, API, predstavitev, iskanje |
| internal/master/web | uporabniški vmesnik (HTML/CSS/JavaScript), jeziki in ta navodila |
| internal/sys, internal/win, internal/screen | funkcije sistema Windows (zajem zaslona, vnos, zaklenjen zaslon, ikona) |

Kako dodati novo funkcijo (primer »počisti odložišče«): 1) v `Agent.Exec` (internal/agent/agent.go) dodajte `case "clear_clipboard":` in ime dodajte v `Features` v proto.go; 2) v seznam `TOOLS` ali v `moreItems()` v web/js/app.js dodajte gumb, ki pokliče `act('clear_clipboard')`; 3) besedila dodajte v vse datoteke v web/i18n. Starejši agenti odgovorijo »unknown action«, zato je mešanje različic varno. Pred prevajanjem zaženite `go test ./...` in `python3 tools/checki18n/check.py`.

## 15. Primerjava s programom Veyon – kaj je drugače

Orbit Classroom pokriva vsakdanje funkcije programa Veyon Master za pouk in dodaja sodobne:

| Funkcija | Orbit Classroom 1.0 |
|---|---|
| Sličice vseh zaslonov | da – prenašajo se samo spremenjene slike |
| Oddaljeni ogled / upravljanje | da – plus izbira monitorja, snemanje videa, vnos besedila, prikaz pasovne širine v živo |
| Predstavitev čez cel zaslon / v oknu | da – plus deljenje zaslona učenca z razredom |
| Zaklepanje zaslonov | da – plus zaklepanje samo vnosa in lastno besedilo |
| Besedilna sporočila | da – plus predloge in sporočila učenec → učitelj |
| Vklop / ponovni zagon / zaustavitev / odjava | da – plus opozorilo 60 s in preklic |
| Zagon programa / odpiranje spletne strani | da – plus priljubljene |
| Prenos datotek, posnetki zaslona | da – plus galerija posnetkov |
| Učilnice (lokacije) | da – vlečenje in spuščanje, vrstni red sedežev, vgrajeno samodejno iskanje po omrežju |
| Novo | ankete v živo, dvig roke, sporočila učencev, način zbranosti, dejavnost in čas nedejavnosti, poudarek, ukazna paleta, temna/svetla tema, angleščina + slovenščina z jezikovnimi paketi, vgrajena navodila, dejanja po meri, API za avtomatizacijo, namestitveni paket z enim klikom |
| (Še) ni na voljo | oddaljena prijava uporabnika, ogled prijavnega zaslona Windows, preden je uporabnik prijavljen, računalniki z Linuxom/macOS, uvoz iz LDAP/Active Directory (glejte 16. poglavje) |

## 16. Znane omejitve in načrt razvoja

Omejitve različice 1.0:

- Agent teče v seji prijavljenega uporabnika. Preden se kdo prijavi, je računalnik prikazan kot nepovezan (vklop, ponovni zagon in zaustavitev prijavljenih računalnikov delujejo).
- Učenec lahko agenta konča z upraviteljem opravil; računalnik je nato do naslednje prijave prikazan kot nepovezan.
- Samo en prijavljen uporabnik na računalnik (ni strežnikov za oddaljeno namizje).
- Oken UAC, zaslona Ctrl+Alt+Del in programov s skrbniškimi pravicami ni mogoče upravljati (varnost sistema Windows).
- Samo Windows 10/11.
- To je prva različica: preizkusite jo v eni učilnici, preden jo uvedete v celi šoli.

Načrtovani / možni naslednji koraki: način storitve Windows (prijavni zaslon, oddaljena prijava, zaščita pred zapiranjem), uvoz učilnic iz Active Directory/CSV, načrtovana dejanja (npr. zaklepanje ob koncu ure – že zdaj mogoče z API-jem), zbiranje datotek od učencev, blokiranje spletnih strani, agent za Linux, več jezikov.

## 17. Pregled ukazne vrstice

OrbitAgent.exe:

| Možnost | Pomen |
|---|---|
| (brez) | zagon; prvi zagon prikaže okno za nastavitev |
| --install | namestitev za vse uporabnike (zahteva skrbniške pravice) |
| --install --silent | namestitev brez oken (skripte, GPO, Intune) |
| --key OPK1-… | ključ(i) učilnice, ločeni z vejico (z --install) |
| --uninstall [--silent] | odstranitev s tega računalnika |
| --portable | zagon brez namestitve |
| --port N | druga vrata TCP |
| --lang sl | jezik besedil za učence |
| --config datoteka | uporabi določeno datoteko z nastavitvami |

OrbitMaster.exe:

| Možnost | Pomen |
|---|---|
| (brez) | zagon in odprtje okna (drugi zagon samo odpre novo okno) |
| --no-window | delovanje brez okna (API za avtomatizacijo) |
| --port N | lokalna vrata okna/API-ja (privzeto 21480) |
| --dir mapa | uporabi drugo mapo z nastavitvami (npr. na USB ključu) |

## 18. Slovarček

| Izraz | Pomen |
|---|---|
| Agent | majhen program na vsakem učenčevem računalniku |
| Master | učiteljev program |
| Sistemska vrstica (tray) | ikone ob uri spodaj desno na zaslonu |
| Učiteljev ključ | digitalni ključ, ki dokazuje, da smete upravljati računalnike |
| Prstni odtis | kratka oznaka ključa za primerjavo ključev |
| Namestitveni paket | datoteka zip z agentom in vašim javnim ključem |
| Wake-on-LAN (WOL) | vklop računalnika prek omrežja |
| Naslov MAC | enolični strojni naslov omrežne kartice (potreben za WOL) |
| Naslov IP | omrežni naslov računalnika, npr. 192.168.1.21 |
| Oddajanje (broadcast) | omrežno sporočilo vsem računalnikom hkrati (za iskanje) |
| Požarni zid | zaščita sistema Windows, ki blokira omrežne povezave |
| Okno UAC | okno sistema Windows z vprašanjem »Ali želite tej aplikaciji dovoliti spremembe?« |
| SmartScreen | opozorilo sistema Windows za nove ali nepodpisane programe |
| TLS | šifriranje, ki ga uporabljajo tudi spletne banke |
| API | vmesnik, prek katerega lahko drugi programi upravljajo Orbit |
| Delta ploščice | tehnika, s katero Orbit pošilja samo spremenjene dele zaslona |
