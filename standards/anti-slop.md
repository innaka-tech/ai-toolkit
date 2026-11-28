# Anti-slop: no generic AI output

AI-slop is output that looks finished but carries no thought: filler words, decoration, code that narrates itself, errors swallowed to make something run, and interfaces that look like every template. Every line must earn its place for this project, these users, and this problem. `aitk slop` finds the mechanical cases; `/aitk-deslop` reviews and fixes the rest.

<!-- aitk-slop: ignore-file (this standard quotes the patterns it bans) -->

## Code
- No comments that restate the code ("increment i", "return the result"). A comment says why, in one line, or it goes.
- No swallowed errors (`catch {}`, `except: pass`, `_ = err`) to make something run. Handle it, return it with context, or log it and say why it is safe.
- No speculative abstraction: no interface with one implementation, factory for one type, config flag nobody sets, or layer that only forwards calls. Add it when the second case exists.
- No defensive noise: no null checks for values that cannot be null, no try/catch around code that cannot throw, no duplicated validation in every layer. Validate at the trust boundary once.
- No placeholder leftovers: `TODO: implement`, sample keys, lorem ipsum, commented-out code, unused imports and parameters.
- Names come from the domain (shipment, waybill, picker), not from the pattern (manager, helper, handler, util, data).

## Text (docs, commit messages, PR descriptions, UI copy, replies)
- No filler or marketing words: seamless, robust, cutting-edge, delve, leverage, unlock, elevate, game-changer, best-in-class. Say what it does and the number that proves it.
- No assistant voice in files: "Certainly!", "I hope this helps", "As an AI".
- No emoji in headings or bullets, no bold on every other phrase, no exclamation marks.
- No summary that repeats the text above it; no intro that restates the question.
- No claim without evidence: "fast" needs a measurement, "secure" needs the control, "tested" needs the test.

## UI
- No generic template look: gradient hero, glassmorphism cards, rainbow buttons, stock illustrations, three equal feature cards, everything centred.
- One primary action per screen, labelled with a verb; real content and real data shapes in designs, never lorem ipsum.
- A visual direction chosen for the product and its users (see `/aitk-creative`), applied consistently with the design system's tokens.

## How to apply
1. Run `aitk slop` on the change; fix every hit or mark a deliberate one with `aitk-slop: ignore` on that line.
2. Read the diff once more asking, line by line: would a senior engineer on this team have written this? Delete what fails.
3. Smaller is the usual result. If the fix makes the code longer, check that it is handling a real case, not adding ceremony.
