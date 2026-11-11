# Orbit Classroom – User Guide

Version 1.0 · English · This guide is also built into Orbit Master (left menu → **Help & guide**). A Slovenian version is in NAVODILA_SL.txt and in the app.

Orbit Classroom lets a teacher see and manage all student computers in a classroom: live thumbnails, presenting your screen, locking screens, remote control, messages, polls, sending files, starting programs and websites, screenshots, focus mode and power control.

> Tip: everything in this guide that may be hard to understand is explained in plain words. Technical details are marked "for IT staff".

## 1. Quick start (about 10 minutes)

1. Copy **OrbitMaster.exe** to the teacher computer and double-click it. A window opens. (Windows may show a blue "Windows protected your PC" warning the first time – click **More info → Run anyway**. See chapter 4.)
2. Choose your language in the welcome tour.
3. Click **Download setup package** (in the tour, in *Find computers* or in *Settings → Teacher key*). You get **Orbit-Agent-Setup.zip**.
4. Copy the zip to every student computer (USB stick or a network share), unzip it and double-click **Install-Orbit-Agent.cmd**. Confirm the Windows administrator prompt. That's all on the student side – the agent now starts automatically for every user.
5. Back on the teacher computer: open **Find computers** and click **Search network**. Select the computers and click **Add selected**.
6. Done. Click a computer to select it, double-click it to watch it live, or use the toolbar buttons for the whole room.

## 2. The two programs

| Program | Where | What it does |
|---|---|---|
| **OrbitMaster.exe** | teacher computer | The console you work with. No installation needed – just start it. It contains the student program, so it can create the setup package for you. |
| **OrbitAgent.exe** | every student computer | Runs invisibly in the background (small Orbit icon next to the clock). Sends screen pictures to the teacher and carries out the teacher's commands. Installed once. |

Files in the download:

| File | Purpose |
|---|---|
| OrbitMaster.exe | teacher program |
| OrbitAgent.exe | student program (also included inside OrbitMaster.exe) |
| GUIDE_EN.txt / NAVODILA_SL.txt | this guide in English / Slovenian |
| README-FIRST.txt | one page quick start (English + Slovenian) |
| lang\ | folder for additional languages (see 14.1) |

Requirements: Windows 10 or Windows 11 (64-bit; an ARM64 build is included). Microsoft Edge (pre-installed on every Windows 10/11) is used to draw the teacher window. All computers must be in the same network (see chapter 6).

## 3. Installing Orbit Agent on student computers

### 3.1 One computer at a time

1. Copy the unzipped **Orbit Agent Setup** folder to the computer.
2. Double-click **Install-Orbit-Agent.cmd** (or OrbitAgent.exe).
3. Confirm the administrator prompt (you need an administrator password once).
4. A message confirms the installation. The Orbit icon appears next to the clock.

What the installer does (for IT staff):

- copies the program to `C:\Program Files\Orbit Classroom\OrbitAgent.exe`
- writes the configuration to `C:\ProgramData\Orbit Classroom\agent.json` and creates a TLS certificate there; the folder is set to *read-only for students* (Administrators/SYSTEM full control, Users read)
- **registers a protected Windows service** `OrbitAgentSvc` running as LocalSystem, start type automatic, with restart-on-crash recovery actions. The service supervises the user-session agent and relaunches it within ~2 seconds if it stops (see 3.6)
- adds a Windows Firewall rule "Orbit Agent" for the program, inbound **and** outbound, on every profile
- adds an entry to **Settings → Apps** so it can be uninstalled normally

### 3.2 Many computers at once (silent installation)

Run as administrator or as SYSTEM (Group Policy startup script, Intune Win32 app, PDQ Deploy, SCCM, a script…):

```
OrbitAgent.exe --install --silent
```

The key is read from **orbit-agent.json** in the same folder. You can also pass the key directly (copy the full line from *Find computers → Set up student computers*):

```
OrbitAgent.exe --install --silent --key OPK1-XXXX-XXXX-...
```

With `--silent` no windows are shown. The protected service starts immediately and launches the agent in the current session; it also runs for every future user. Exit code 0 = success.

Extra flags for mass deployment:

| Flag | Meaning |
|---|---|
| `--key OPK1-…` | the classroom key (or several, comma separated) |
| `--no-service` | install the lightweight way (autostart only, no protected service — a student could then close the agent until the next sign-in) |
| `--port N` | use a different TCP port |
| `--lang sl` | force the student language |

### 3.3 Trying it without installing

Double-click OrbitAgent.exe and choose **Run once**. It runs until the user signs out, and the tray menu contains **Exit**. Good for testing; for school use always install.

### 3.4 Updating to a new version

Run the new OrbitAgent.exe with `--install` (or the new setup package) – it replaces the old program and keeps the configuration. Orbit Master and Orbit Agent of the same major version always work together.

### 3.5 Removing

**Settings → Apps → Orbit Agent (Orbit Classroom) → Uninstall**, or `OrbitAgent.exe --uninstall` (add `--silent` for scripts). Uninstalling stops and deletes the service, removes the firewall rule and deletes the program and its configuration. Administrator rights are required (the program asks for them).

### 3.6 Protected mode – why a student cannot turn it off

By default the agent is installed as a **Windows service** (`OrbitAgentSvc`). This matters in a classroom:

- The service runs as the computer's **SYSTEM** account and starts **before** anyone signs in. Stopping or deleting a Windows service needs administrator rights, so a student (a standard user) cannot switch the monitoring off from Services, `sc stop`, or the Task Manager "Services"/"Startup" tabs.
- If a student ends the agent from Task Manager, the service notices and **relaunches it within about two seconds**, so the teacher only loses the screen for a moment.
- If the service itself ever crashes, Windows restarts it automatically (recovery actions).
- The program lives in `C:\Program Files\...`, which a standard user cannot modify or delete, and its configuration folder is read-only for them.

Nothing is hidden: the agent still shows the tray icon and the "your teacher is viewing your screen" notifications exactly as before. Protected mode only changes **who may turn it off** (an administrator), not **whether the student can tell it is there** (they always can).

If you would rather have the lightweight behaviour – the agent starts at sign-in but a student could close it until the next sign-in – install with `--no-service`. This can be useful on teachers' own laptops or for quick trials.

> Note on the Windows sign-in screen: the agent shows the teacher a screen only while a user is signed in, because screen capture must run inside that user's session. The service keeps the agent alive and relaunches it, but it cannot display the Windows password screen itself. Power-on, restart and shutdown of signed-in computers work regardless.

## 4. Windows security warnings – what they mean

**"Windows protected your PC" (SmartScreen).** Programs that are not digitally signed with a paid certificate, or that are new, trigger this blue warning. Click **More info → Run anyway**. Your IT department can avoid it by signing the .exe files with the school's code-signing certificate, or by allowing them in Windows Defender / Intune.

**Antivirus.** Programs that can view and control screens are sometimes flagged by antivirus software because remote-control tools can be misused. If OrbitAgent.exe is blocked, add an exception for `C:\Program Files\Orbit Classroom\`.

**Firewall.** The installer opens the Windows Firewall for OrbitAgent.exe automatically, inbound and outbound, on every network profile (domain, private and public), so the teacher can always reach the computer. If your school uses a different firewall product, allow TCP 21400 and UDP 21401 for OrbitAgent.exe.

**Administrator prompt.** Only the *installation* of the agent needs administrator rights. Orbit Master never needs them.

## 5. The teacher key – explained simply

Think of the key as a **key and lock**:

- The **secret teacher key** (starts with `OTK1-`) is the key. It stays inside Orbit Master on your computer.
- The **public key** (starts with `OPK1-`) is the matching lock. It is built into the setup package and installed on every student computer.
- A student computer only obeys an Orbit Master whose key fits its lock. Students cannot use the public key to control other computers – it only opens nothing.

Technically (for IT staff): Ed25519 signatures with a fresh challenge for every login, bound to the agent's TLS certificate (protects against replay and man-in-the-middle relays); all traffic is TLS-encrypted.

Important situations:

| Situation | What to do |
|---|---|
| Several teachers should control the same rooms | On the first teacher computer: *Settings → Teacher key → Back up secret key*. On the other teacher computers: *Import secret key*. |
| New teacher computer / reinstalled Windows | Import your backed-up secret key – no need to touch the student computers. |
| Secret key lost (no backup) | *Generate new key*, then install the new setup package on all student computers (the installer adds the new key). |
| Secret key may have been stolen | *Generate new key* and reinstall the agents; then remove the old key from `agent.json` (see 14.4). |
| A computer shows "Key not accepted" | It was installed with a different key. Install your setup package on it. |

The fingerprint (e.g. `4E18-83B9-7FD0`) is a short "name" of your key, so you can compare keys quickly.

## 6. Network requirements

| Port | Direction | Used for |
|---|---|---|
| TCP 21400 | teacher → student | everything (encrypted) |
| UDP 21401 | teacher → student (broadcast) | finding computers automatically |
| UDP 9 and 7 | teacher → network (broadcast) | Wake-on-LAN (power on) |
| TCP 21480 | only inside the teacher computer (127.0.0.1) | the teacher window and the automation API |

- The teacher computer only makes **outgoing** connections – nothing has to be opened on it.
- Teacher and student computers should be in the same network (same building / VLAN). Automatic search uses broadcasts, which do not cross routers. In other networks add computers by name or IP address, or use the *deep scan*.
- **Wake-on-LAN** (power on) only works if it is enabled in the BIOS/UEFI and network adapter settings, the computer is connected by cable, and is in the same network. The MAC address is learned automatically the first time a computer is online.
- Bandwidth: thumbnails are small and only sent when the picture changes. Live view and demo send only the changed parts of the screen; a static slide costs almost nothing. For 30 computers a normal 1 Gbit/s school network is plenty. On slow networks lower the demo frame rate (Settings → Speed & quality).

## 7. Rooms and computers

- **Rooms** (left menu) group computers – e.g. one room per computer lab. Create one with **+**. Rename or delete with the **⋯** button that appears when you point at a room. Deleting a room only removes it from Orbit Master.
- **Find computers** → **Search network** lists every running Orbit Agent. Column *Your key*: **trusted** means it was installed with your key; **other key** means it belongs to another teacher key. New computers are pre-selected – choose a room and click **Add selected**.
- **Deep scan** tries every address of your local network – use it if the normal search finds nothing (some networks block broadcasts).
- **Add manually**: enter the computer name (e.g. `PC-12`) or IP address.
- **Edit**: right-click a computer → *Edit computer* (name, address, room, MAC address).
- **Reorder**: drag a computer onto another to place it before it – arrange tiles like the seats in the room. **Move to another room**: drag tiles onto a room in the left menu.

Status dots: green = online, yellow = connecting, grey = offline, red = key not accepted or incompatible version.

## 8. Working in class – all functions

### 8.1 Which computers does an action affect?

Look at **Applies to** above the tiles:

- If you **selected** computers, actions affect only those.
- If nothing is selected, actions affect **all online computers that are shown** (respecting the filter chips and the filter box).

Selecting: click = select one · Ctrl+click = add/remove · Shift+click = range · drag on empty space = rubber-band selection · Ctrl+A = all · Esc = clear.

The **filter chips** (All, Online, Offline, Raised hands, Locked) and the **filter box** (name, user or program) limit what is shown. The **slider** changes the tile size.

### 8.2 Thumbnails and activity

Every tile shows the live screen, computer name, signed-in user, the active program and how long the student has been idle (yellow after 5 minutes). Badges: red **Live** (screen viewed or controlled right now), lock, demo, focus, and a yellow waving hand for raised hands. Activity display can be turned off in Settings.

**Spotlight** (More → Spotlight, or key S): shows only the selected computers, large and with a faster refresh rate.

### 8.3 Demo (presenting)

- **Full screen demo**: students only see your screen; their keyboard and mouse are blocked.
- **Window demo**: your screen appears in a window – students can keep working.
- **Share a student's screen**: shows one student's screen to all other computers (great for showing a good solution).
- A bar at the top shows the running demo; **Stop** ends it.

Your main monitor is shown. Students who join later get the picture within a few seconds.

### 8.4 Lock

- **Lock screens** (key L): a full screen "Screen locked" picture; keyboard and mouse are blocked.
- **Lock mouse & keyboard only**: the screen stays visible but cannot be used (e.g. while you explain what is on their screen).
- **Lock with custom text**: your own title and text. A default can be set in Settings.
- Click the button again (it now says **Unlock**) to unlock. Locks stay active even if Orbit Master is closed.

Note: Windows always allows Ctrl+Alt+Del – it cannot be blocked by any program. Students who press it only see the Windows security screen and can return.

### 8.5 View and control

Double-click a tile (or select one and press V / C). The viewer opens:

- **View / Control** switch. In *Control* mode your mouse and keyboard work on the student computer.
- Buttons for **Win**, **Alt+Tab**, **Ctrl+Shift+Esc** (keys that your own Windows would otherwise catch) and **Type text** (types any text, including č, š, ž).
- **Monitor** choice when the student has several screens. **Quality**: auto/low/medium/high. **Fit** switches between fitting the window and actual size.
- **Screenshot**, **Record video** (saved as .webm into your Downloads folder), **Full screen** (in full screen also keys like Ctrl+W are sent to the student).
- The top right shows resolution, frames per second and bandwidth.
- In view mode Esc closes the viewer; in control mode use the ✕ button (Esc goes to the student).

The student sees a notification and a red dot on the tray icon while being viewed or controlled.

Windows protects some screens: the **UAC administrator prompt**, the **Ctrl+Alt+Del screen** and the **Windows lock screen** cannot be viewed or controlled (the picture freezes and input is ignored). Programs running "as administrator" on the student computer cannot be controlled either. This is a Windows security feature.

### 8.6 Message

Sends a message window to students (key M). Ready-made templates are offered. The optional title replaces "Message from your teacher".

### 8.7 Poll

Ask a question with up to 12 answers (key P). Presets: Yes/No, A–D, 1–5, understanding, true/false, pace. Students answer by clicking a button in a pop-up window. The **Polls** page shows a live chart, every student's answer and who has not answered yet. **Close poll** removes the window on student computers; **CSV** exports the results (opens in Excel).

### 8.8 Website and Run program

- **Website** opens a web address in the student's default browser (only http:// and https://).
- **Run** starts a program, document or command as the signed-in student – like a double-click (e.g. `notepad.exe`, `winword.exe`, `C:\Apps\Geogebra\GeoGebra.exe`, or a document path). Arguments are optional.
- Both dialogs offer **favorites**; tick *Save to favorites* or edit them in Settings.

### 8.9 Send file

Drop a file into the dialog. It is copied to every selected computer into the folder **Desktop\Orbit** (an existing file is not overwritten – "(2)" is added). Optionally the file is opened automatically.

### 8.10 Screenshot

Saves full-resolution PNG pictures of the selected screens into `Pictures\Orbit Screenshots` (configurable). The **Screenshots** page shows them; click to enlarge.

### 8.11 Focus mode

Automatically closes listed programs (games, chat, browsers…) on student computers as long as focus mode is on. Presets add common programs. Students get a notification. Programs are closed **without asking**, so unsaved work in them is lost. Turn it off in the same dialog.

### 8.12 Power

- **Power on** (Wake-on-LAN) for offline computers – see the requirements in chapter 6.
- **Restart**, **Shut down**, **Log off**. With *Give students a 60 second warning* Windows shows a countdown message; **Cancel pending shutdown** stops it.
- Asking for confirmation can be turned off in Settings.

### 8.13 Raised hands and student messages

Students click the Orbit icon next to the clock → **Raise hand** (or press **Ctrl+Alt+H**), or **Message to teacher…**. You hear a sound and see a notification; the tile shows a waving hand. The **Inbox** page lists raised hands (oldest first) and messages; you can reply, view the screen or lower the hand.

### 8.14 Command palette

Press **Ctrl+K** and type: any action, room, computer, setting or language. Arrow keys + Enter to run. The fastest way to work.

## 9. Keyboard shortcuts

| Key | Action |
|---|---|
| Ctrl+K | command palette |
| Ctrl+A | select all shown computers |
| Esc | clear selection / close |
| L / U | lock / unlock |
| M | message |
| P | poll |
| V / C | view / control (one selected computer) |
| D | demo menu |
| S | spotlight on/off |
| + / − | larger / smaller tiles |
| Alt+1 … Alt+9 | switch room |
| ? | this guide |
| Ctrl+Enter | confirm the open dialog |

Students: **Ctrl+Alt+H** raises or lowers the hand.

## 10. What students see (privacy and transparency)

- An Orbit icon next to the clock. Its dot shows: **red** = the teacher is viewing/controlling right now, **yellow** = hand raised, **grey** = locked.
- A Windows notification when the teacher starts viewing or controlling the screen, when the screen is shown to the class, and when focus mode closes a program.
- *About Orbit Agent* in the tray menu explains in plain words what the teacher can see.

Recommendations: inform students and parents in your school's IT rules that classroom computers are monitored during lessons, and what is recorded. Orbit does not record anything by itself – screenshots and video recordings only happen when the teacher clicks the buttons, and are stored on the teacher computer only. (For IT staff: the agent setting `notify_on_view` can only be changed by administrators.)

## 11. Settings reference

| Setting | Meaning |
|---|---|
| Language | language of the teacher program (student programs use the Windows language) |
| Appearance / Accent colour | dark, light or system theme; colour of buttons and highlights |
| Sounds | sound for raised hands and messages |
| Show student activity | active program and idle time on tiles |
| Confirm power actions | ask before restart / shut down / log off |
| Thumbnail refresh | how often tiles update (default 1 s) |
| Remote view quality / frame rate / max. width | picture quality vs. bandwidth of the viewer |
| Demo quality / frame rate / max. width | same for demos (lower frame rate on slow networks) |
| Lock screen | default title and text on locked screens |
| Teacher key | public key, setup package, backup/import/new key (chapter 5) |
| Favorites | websites and programs for the dialogs |
| Custom actions | your own buttons (14.2) |
| Automation API | token for scripts (14.3) |
| Files & folders | configuration folder, screenshot folder |

## 12. Troubleshooting

| Problem | Solution |
|---|---|
| "Windows protected your PC" | More info → Run anyway (chapter 4). |
| Search finds nothing | Is the agent installed and the Orbit icon visible on the student computer? Same network? Try *Deep scan*. Check firewall/antivirus. Add the computer by IP address. |
| Tile says **Offline** / "Orbit Agent is not running" | Computer off, nobody signed in yet (the agent starts at sign-in), or the agent was closed. Sign in or restart the computer. |
| Tile says **Key not accepted** | The computer was installed with another teacher key – install your setup package (chapter 5). |
| Tile says **Incompatible version** | Update Orbit Master and Orbit Agent to the same version. |
| No answer / timeout | Firewall blocks TCP 21400, or the computer is in another network. |
| Picture is black or frozen | A UAC prompt or the Windows lock screen is shown on the student computer (chapter 8.5). |
| Control does not work in one program | That program runs as administrator (chapter 8.5). |
| Power on does nothing | Wake-on-LAN disabled in BIOS / network adapter, Wi-Fi only, or another network (chapter 6). |
| Teacher window does not open | Microsoft Edge is missing or blocked – Orbit then opens your default browser. You can also open the address from the log file. |
| Two teacher computers, one is rejected | Import the same secret key on both (chapter 5). |
| Student closed the agent | The computer shows offline. A restart or sign-out/sign-in starts it again. |
| "Port 21400 is already in use" | Another Orbit Agent (another signed-in user on the same PC) is running. Only one session per computer is supported. |

Log files: `%APPDATA%\Orbit Classroom\logs\master.log` (teacher) and `%APPDATA%\Orbit Classroom\logs\agent.log` (student, per user). Type the path into the Explorer address bar.

## 13. Files and folders

| Path | Content |
|---|---|
| `%APPDATA%\Orbit Classroom\master.json` | teacher settings, rooms, computers **and the secret key** – keep the teacher Windows account protected with a password |
| `%APPDATA%\Orbit Classroom\logs\` | log files |
| `%APPDATA%\Orbit Classroom\ui-profile\` | private browser profile of the teacher window (can be deleted) |
| `Pictures\Orbit Screenshots\` | screenshots |
| `C:\Program Files\Orbit Classroom\` | installed agent |
| `C:\ProgramData\Orbit Classroom\agent.json` | agent configuration (read-only for students) |
| `C:\ProgramData\Orbit Classroom\agent-cert.pem / agent-key.pem` | agent TLS certificate |
| `Desktop\Orbit\` (student) | received files |

## 14. Extending Orbit Classroom (options for the future)

Orbit is designed so you can improve it without being a programmer for the common cases, and with full source code for everything else.

### 14.1 Add a language

1. Copy `en.json` from the source folder `internal\master\web\i18n\` (or ask for it) and translate the texts on the right side. Keep `{n}`, `{name}` etc. unchanged. Set `"_name"` to the language name, e.g. `"Deutsch"`.
2. Save it as `lang\de.json` next to OrbitMaster.exe (use the two-letter language code). Restart Orbit Master – the language appears in Settings.
3. Optional: put a translated guide as `lang\de.md` next to OrbitMaster.exe – it appears under Help.
4. Optional for students: create `agent-de.json` next to OrbitAgent.exe (installed: `C:\Program Files\Orbit Classroom\`) with the agent texts (keys listed in `internal\agent\i18n.go`). The agent uses the Windows display language automatically.

Missing texts automatically fall back to English, so partial translations work.

### 14.2 Custom actions

Settings → Custom actions → **Add action**. Types: *Run program* (program + arguments), *Open website* (URL), *Send message* (text). They appear in **More** and in the command palette (Ctrl+K). Example: name "GeoGebra", type Run, value `C:\Program Files\GeoGebra\GeoGebra.exe`.

### 14.3 Automation API (for IT staff)

Orbit Master offers a local REST API on `http://127.0.0.1:21480/api` (only reachable from the teacher computer itself). Create a token in Settings → Automation API and send it as `Authorization: Bearer <token>`.

| Method & path | Purpose |
|---|---|
| GET /api/state | rooms, computers with live status, inbox, poll, demo |
| POST /api/action | `{"targets":[ids],"action":"lock","params":{...}}` – actions: lock, unlock, message, open_url, run, power, focus, hand_lower, demo_stop |
| POST /api/wake | `{"targets":[ids]}` Wake-on-LAN |
| POST /api/demo/start · /api/demo/stop | `{"source":"teacher" or id,"mode":"fullscreen" or "window","targets":[ids]}` |
| POST /api/poll · /api/poll/close | `{"targets":[ids],"question":"…","options":["Yes","No"]}` |
| POST /api/screenshot | `{"targets":[ids]}` |
| POST /api/file | multipart form: targets (JSON), open (0/1), file |
| POST /api/computers · PUT/DELETE /api/computers/{id} | manage computers |
| POST /api/rooms · PUT/DELETE /api/rooms/{id} | manage rooms |
| POST /api/discover | `{"scan":false}` search the network |
| GET /api/thumb/{id} | current thumbnail (JPEG) |

Parameters of `params`: lock → mode ("screen"/"input"), title, text · message → title, text · open_url → url · run → program, args · power → mode ("reboot", "shutdown", "logoff", "abort"), delay (seconds), text · focus → enabled, list (program names).

PowerShell example (e.g. lock every room at the end of a lesson with the Windows Task Scheduler):

```
$h = @{ Authorization = "Bearer orb_your_token" }
$s = Invoke-RestMethod -Headers $h http://127.0.0.1:21480/api/state
$ids = $s.computers | Where-Object { $_.state -eq "online" } | ForEach-Object { $_.id }
$body = @{ targets = $ids; action = "lock" } | ConvertTo-Json
Invoke-RestMethod -Method Post -Headers $h -ContentType "application/json" -Body $body http://127.0.0.1:21480/api/action
```

Orbit Master must be running. Start it headless with `OrbitMaster.exe --no-window` for unattended automation.

### 14.4 Agent options (agent.json, for IT staff)

Edit `C:\ProgramData\Orbit Classroom\agent.json` as administrator (or edit `orbit-agent.json` in the setup package before installing). Changes apply after the next sign-in.

| Option | Default | Meaning |
|---|---|---|
| trusted_keys | your key | list of public teacher keys (OPK1-…) that may control this computer |
| port | 21400 | TCP port |
| discovery | true | answer automatic network search |
| language | auto | student language: auto, en, sl, … |
| notify_on_view | true | notify students when viewed/controlled |
| allow_control | true | allow remote mouse & keyboard (false = view only) |
| show_tray | true | show the Orbit tray icon |
| allow_hand | true | raise-hand feature |
| allow_messages | true | student → teacher messages |
| share_activity | true | report active program and idle time |
| received_folder | (Desktop\Orbit) | folder for received files |

### 14.5 Building from source (for developers)

The complete source code is in OrbitClassroom-1.0.0-source.zip. It is written in Go (no other tools or libraries needed):

```
build.cmd          (on Windows: needs Go from https://go.dev/dl/)
./build.sh         (on Linux/macOS: builds x64 + ARM64 packages with icons)
```

Project layout:

| Folder | Content |
|---|---|
| cmd/master, cmd/agent | program entry points |
| internal/proto | shared protocol: keys, login, screen stream format, discovery |
| internal/agent | student agent: API server, commands, installer, texts |
| internal/master | teacher console: connections, API, demo, discovery |
| internal/master/web | the user interface (HTML/CSS/JavaScript), languages and this guide |
| internal/sys, internal/win, internal/screen | Windows functions (screen capture, input, lock screen, tray) |

How to add a new feature (example "clear clipboard"): 1) add a `case "clear_clipboard":` in `Agent.Exec` (internal/agent/agent.go) and its name to `Features` in proto.go; 2) add a button in the `TOOLS` list or in `moreItems()` in web/js/app.js that calls `act('clear_clipboard')`; 3) add the texts to every file in web/i18n. Old agents answer "unknown action", so mixing versions is safe. Run `go test ./...` and `python3 tools/checki18n/check.py` before building.

## 15. Compared with Veyon – what is different

Orbit Classroom covers the everyday classroom functions of Veyon Master and adds modern ones:

| Function | Orbit Classroom 1.0 |
|---|---|
| Thumbnails of all screens | yes – only changed pictures are transferred |
| Remote view / remote control | yes – plus monitor choice, video recording, typing text, live bandwidth display |
| Demo full screen / window | yes – plus sharing a student's screen with the class |
| Lock screens | yes – plus lock input only and custom lock text |
| Text messages | yes – plus templates and student → teacher messages |
| Power on / restart / shut down / log off | yes – plus 60 s warning and cancel |
| Run program / open website | yes – plus favorites |
| File transfer, screenshots | yes – plus screenshot gallery |
| Rooms (locations) | yes – drag & drop, seat order, automatic network search built in |
| New | protected service mode (a student cannot close the agent), live polls, raise hand, student messages, focus mode, activity & idle time, spotlight, command palette, twelve colour themes (dark & light) + accent colours, English + Slovenian with language packs, a plain-language "Simple" guide plus the full guide, custom actions, automation API, one-click setup package |
| Not (yet) available | signing in a user remotely, viewing the Windows sign-in screen before a user is signed in, Linux/macOS computers, LDAP/Active Directory import (see chapter 16) |

## 16. Known limitations and roadmap

Limitations of version 1.0:

- The agent captures the screen inside the signed-in user's session. Before anyone signs in, the computer shows no screen (power on, restart and shutdown of signed-in computers work). The protected service keeps the agent alive once a user signs in and relaunches it if a student closes it, but it cannot display the Windows sign-in (password) screen itself.
- Only one signed-in user per computer (no Remote Desktop servers).
- UAC prompts, the Ctrl+Alt+Del screen and elevated programs cannot be controlled (Windows security).
- Windows 10/11 only.
- This is a first version: test it in one room before rolling it out to the whole school.

Planned / possible next steps: showing and signing in at the Windows sign-in screen, Active Directory/CSV import of rooms, scheduled actions (e.g. lock at the end of the lesson – already possible with the API), collecting files from students, website blocking, Linux agent, more languages.

## 17. Command-line reference

OrbitAgent.exe:

| Option | Meaning |
|---|---|
| (none) | start; the first start shows the setup window |
| --install | install for all users as a protected service (asks for administrator rights) |
| --install --silent | install without any window (scripts, GPO, Intune) |
| --install --no-service | install the lightweight way (autostart only, a student can close it) |
| --key OPK1-… | classroom key(s), comma separated (with --install) |
| --uninstall [--silent] | remove from this computer |
| --portable | run without installing |
| --port N | other TCP port |
| --lang sl | language of the student texts |
| --config file | use a specific configuration file |

(`--service` is used internally by Windows to run the background service; you never type it yourself.)

OrbitMaster.exe:

| Option | Meaning |
|---|---|
| (none) | start and open the window (a second start only opens another window) |
| --no-window | run without window (automation API) |
| --port N | local port of the window/API (default 21480) |
| --dir folder | use another configuration folder (e.g. on a USB stick) |

## 18. Glossary

| Term | Meaning |
|---|---|
| Agent | the small program on each student computer |
| Master | the teacher program |
| Tray / system tray | the icons next to the clock at the bottom right of the screen |
| Teacher key | the digital key that proves you are allowed to control the computers |
| Fingerprint | short identifier of a key, to compare keys |
| Setup package | zip file with the agent and your public key |
| Wake-on-LAN (WOL) | switching a computer on over the network |
| MAC address | unique hardware address of a network card (needed for WOL) |
| IP address | network address of a computer, e.g. 192.168.1.21 |
| Broadcast | a network message to all computers at once (used for searching) |
| Firewall | Windows protection that blocks network connections |
| UAC prompt | the Windows window asking "Do you want to allow this app to make changes?" |
| SmartScreen | the Windows warning for new or unsigned programs |
| TLS | the encryption also used by banking websites |
| API | an interface that lets other programs control Orbit |
| Delta tiles | Orbit's technique of only sending the parts of a screen that changed |
