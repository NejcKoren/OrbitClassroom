# Orbit Classroom

Program za upravljanje učilnice (računalniki učencev) za Windows 10/11 – podobno kot Veyon Master, z modernejšim videzom in sodobnimi funkcijami.
Classroom management for Windows 10/11 – similar to Veyon Master, with a modern look and 2026 features.

## Kaj je v projektu / What is included

| Program | Kdo | Opis |
|---|---|---|
| `OrbitMaster.exe` | učitelj | konzola: sličice vseh računalnikov, ogled in upravljanje v živo, predstavitev, zaklepanje, sporočila, ankete, dvig rok, datoteke, programi, spletne strani, posnetki zaslona, način zbranosti, napajanje (Wake-on-LAN, ponovni zagon, izklop, odjava) |
| `OrbitAgent.exe` | učenec | teče v ozadju, namesti se z enim klikom ali tiho (`--install --silent`) za GPO/Intune |

* Slovenščina in angleščina sta vgrajeni; dodatne jezike dodate z datotekami v mapi `lang`.
* Navodila (SL/EN) so vgrajena v program (*Pomoč in navodila* / *Help & guide*) in v `internal/master/web/docs/`.
* Šifrirane povezave (TLS), dostop samo z učiteljevim ključem (Ed25519), učenci vedno vidijo obvestilo ob ogledu.

## Gradnja / Build

```
./build.sh     # Linux/macOS/WSL: testi + paketi za Windows x64 in ARM64 v dist/
build.cmd      # Windows: navadna gradnja v dist\
```

Potrebno: Go 1.24+ (in `go-winres` za ikone, `zip`). Podrobnosti: `README-DEV.md`.

## Dokumentacija / Docs

* `internal/master/web/docs/sl.md` – navodila v slovenščini
* `internal/master/web/docs/en.md` – user guide in English
* `docs/README-FIRST.txt` – hiter začetek / quick start

## Status

Različica 1.0.0. Preizkušeno z Go testi, end-to-end testom Linux (več agentov) in testom pravih `.exe` datotek pod Wine. Pred uvedbo v šoli preizkusite v eni učilnici – nisem ga mogel preizkusiti na pravem računalniku s Windows.
Version 1.0.0. Tested with Go unit tests, a Linux end-to-end test with several agents, and the real `.exe` files under Wine. Not yet tested on real Windows hardware – try one classroom first.
