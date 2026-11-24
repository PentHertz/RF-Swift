---
name: rfswift-mission-board
description: Keep a shared mission board (board.md) in an RF Swift mission - confirmed facts with their evidence, open exploration directions, and hints the operator writes for the agent - so long assessments, several agents and context resets do not lose state or repeat work. Use it at the start and end of every work cycle and whenever a fact is confirmed or a direction is opened or closed.
---

# The mission board

Long RF and hardware assessments outlast a context window, involve several
specialists, and depend on things only the operator knows (where the device is,
which button opens the gate, what the client cares about). The board is a mission
note named `board.md` that holds the state of the work in three parts. Every
agent reads it before acting and updates it after.

## Format

```markdown
# Board: <mission>

## Goal
<what the client wants answered, in one or two lines>

## Facts
- F1 [2026-11-24 10:05Z] Gate remote transmits OOK at 433.92 MHz, ~2 kbaud bursts of 66 bits. Evidence: captures/433.92M_2Msps_..._gate.cs8, note.md#10:05
- F2 [..] ... (from F1) ...

## Open directions
- D1 (from F1) Compare 5 presses: fixed code or rolling? Owner: signal-analyst
- D2 (from F1, F2) ...

## Closed directions
- D0 Survey 868 MHz: nothing from the target (F3)

## Hints from the operator
- (operator writes here; agents never edit this section)
```

## Rules

- **Facts** are confirmed observations, each with its evidence (file or note
  reference) and the facts it builds on. Never write a guess as a fact: write it
  as a direction to check.
- **Open directions** are what is worth exploring next, each derived from facts,
  independent of the others, and specific enough to act on. Keep at most a
  handful; close or merge the rest.
- **Closed directions** record what was tried and the outcome, including dead
  ends, so nobody repeats them.
- **Hints** belong to the operator. Read them first in every cycle and follow
  them; never edit or remove them. If a hint conflicts with the scope, ask.
- Update the board when something changes, not only at the end: a context reset
  must not lose a confirmed fact.

## The work cycle

1. Read the board (`read_note {name: "board.md"}`) and the hints. If there is no
   board, create one from the mission note and the operator's request.
2. Decide: is the goal answered by the facts? If yes, say so and point to the
   facts. If not, pick the most valuable open direction, or open new ones when the
   current ones do not cover what the facts suggest.
3. Work the direction with the right domain skill.
4. Write back: new facts with evidence, the direction closed or updated, new
   directions it opened (`write_note {name: "board.md", body: ...}` after reading
   the current version, so nothing the operator added is lost).

When several agents work in parallel, each takes a different open direction and
writes its owner on it, so two agents do not do the same work.
