---
name: rfswift-findings-qualification
description: How to qualify and write findings in an RF Swift mission - what counts as proof for RF, wireless, RFID and hardware weaknesses, control cases that rule out coincidence, candidate versus verified status, attack chains across interfaces, CVSS v3.1 vectors for adjacent-radio and physical attacks (AV:A, AV:P), and the save_finding fields. Use it before calling save_finding, when reviewing evidence, and when preparing a report.
---

# Qualifying findings

A finding is a claim a client will act on. In RF and hardware work the usual
failure is not a missing vulnerability but an unproven one: a device that "seemed
to react", a capture that shows activity but not the effect, a key that was read
from documentation instead of the device. This skill sets the bar.

## Status: the agent proposes, the operator decides

Findings saved by an agent are stored with status `draft` and origin `agent`
(the findings list marks them "AI"). Only the operator moves a finding to `open`
or `confirmed`, and once they edit a finding it is theirs: an agent cannot change
it again. To correct or extend an operator's finding, save a new candidate
that names it.

In the description, say which level you reached:

| Level | Meaning | What it needs |
|---|---|---|
| Demonstrated | The impact happened end to end | Proof + control case + reproducible steps |
| Observed | The weakness is visible, the impact was not shown | The observation (capture, dump, log) |
| Hypothesis | Inferred from documentation, versions, behaviour | Say so; never present as a finding title |

A hypothesis belongs in the mission note or the board, not in `save_finding`.

## Proof and control

For each claim, write down before testing: what result proves it, and what result
would disprove it. Then keep both results.

| Claim | Proof | Control |
|---|---|---|
| Fixed-code remote can be replayed | Target acts on the replayed capture | Target ignores a capture with one bit changed, and the original remote still works afterwards |
| Rolling code accepts old codes | Target acts on a code captured earlier | Target ignores a code already used after resynchronisation |
| Badge can be cloned | Reader grants access to the clone | Reader denies a blank card of the same type |
| Default or reused card keys | Sectors read with the default key | Same read fails with a wrong key |
| Unauthenticated BLE write changes state | State change observed after the write without pairing | Same write rejected or no change on a paired-only characteristic |
| UART gives a root shell | `id` shows uid=0 from the console log | Console asks for credentials after the fix / on a hardened unit |
| SWD/JTAG readable | Memory read matches the flash dump (same hash) | Read fails with RDP enabled |
| Secret in firmware | Exact value and path in the extracted image | The value works on the device (only if in scope and approved) |
| Wi-Fi PSK weak | Cracked from a captured handshake | The handshake belongs to the in-scope BSSID |

Activity is not proof: a burst on a waterfall, a reply on a bus, an HTTP 200, a
decoder line, or "the light blinked" only show that something happened.

## Evidence that survives review

- Files in the mission workspace (`captures/`, `dumps/`, `logs/`) with names that
  carry frequency, rate and time, plus sha256 sums.
- The exact commands, in order, in the mission note, with times.
- The hardware used (model, firmware), antenna and distance where it matters.
- For IQ: a `.sigmf-meta` with sample rate, centre frequency, datatype, time.
- A recording of the console session when a shell or a prompt is the proof (the
  operator can record terminals in the Workbench).

## Severity and CVSS v3.1

Choose the attack vector from the access the attacker really needs:

- `AV:A` (adjacent): within radio range or on the same link (Wi-Fi, BLE, Zigbee,
  sub-GHz, NFC relay distances, CAN on an exposed connector of a parked vehicle
  counts as physical rather than adjacent unless it is remotely reachable).
- `AV:P` (physical): touching the device (UART, JTAG/SWD, flash, a card that must
  be read at contact distance).
- `AV:N` only when it is reachable over a routed network.

Then `AC:H` when the attack needs conditions outside the attacker's control (a
victim pairing at that moment, a narrow time window), `UI:R` when a user must act
(press the remote so it can be captured), and impact from what was demonstrated,
not what is imaginable. Write the vector, not only the score:
`CVSS:3.1/AV:A/AC:L/PR:N/UI:R/S:U/C:N/I:H/A:N`.

Never invent CVE identifiers or versions. If a component version is known, say
which, and leave CVE matching to the operator.

## Chains

On devices, the serious issue is often a chain: the Zigbee network key is sent in
the clear at join, so captured traffic can be decrypted, so the lock can be
commanded; or a Wi-Fi PSK read from unprotected flash is the same on every unit,
so one bought device opens every customer's network.

- Record each link as its own finding, with its own proof, control case,
  severity and weakness classes. A link can be fixed on its own, and the report
  must still be right if one link is.
- Describe the chain in the finding with the highest end impact (or a dedicated
  "attack chain" finding): the steps in order, each with the finding id that
  proves it, what an attacker needs (radio range, physical access to one unit,
  time), and the demonstrated end result.
- Score the chain finding on its demonstrated end impact; score each link on what
  it allows by itself. Do not raise a link's severity because of the chain.
- A chain is demonstrated only when every step was shown on the target, in order.
  A step taken from documentation or assumed keeps the chain a hypothesis: say
  which step is missing.
- Note the chain on the mission board so the other specialists test the links it
  depends on (for example, a second unit to check that a key is shared).

## Writing it

`save_finding {finding: {...}}` fields:

- `title`: the weakness and the asset, no verdict words:
  "Gate remote uses a fixed code that can be replayed".
- `sev`: crit, high, med or low; `cvss`: the vector.
- `vulnType`: the class (e.g. "Replay", "Missing authentication",
  "Hard-coded credential", "Debug interface enabled").
- `description`: what is wrong and why it matters, and the level reached.
- `observation`: what you did and saw, with file names and times.
- `poc`: the exact steps and commands to reproduce, including the control case.
- `remediation`: concrete for the technology (rolling codes with proper counter
  windows, LE Secure Connections, disabling JTAG with fuses or RDP level, unique
  per-device keys, ...).
- `references`: standards or vendor documents, one per line.
- `src`: the evidence file or note.
- `weakness`: the compliance weakness class ids, comma-separated. Call
  `map_compliance {vuln_type, title}` to see which classes and requirements
  match, or `map_compliance {}` for the list, and set the ones that describe the
  weakness (for example `replay` or `default-credentials, hardcoded-keys`). The
  report then names the EN 18031 mechanism ([AUM], [SCM], ...), the ETSI EN
  303 645 provision, the OWASP IoT category and the IEC 62443-4-2 requirement.
  Do not cite a requirement the mapping does not give; the operator decides on
  conformity claims.

Before saving, check `list_findings` for a duplicate; update your own candidate
(same `id`) rather than creating a second one.
