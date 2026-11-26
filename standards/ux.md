# UX and accessibility

Interfaces are used by tired people on small screens in bad light. Baseline: WCAG 2.1 level AA.

## Accessibility (must)
- Text contrast at least 4.5:1 (3:1 for large text and UI components); never colour alone to convey meaning.
- Every control reachable and operable by keyboard, in a logical order, with a visible focus indicator; focus trapped in and restored after dialogs.
- Every input has a visible label; errors are announced and say how to fix the problem; images have alt text (empty for decorative ones).
- Semantic HTML first, ARIA only where HTML cannot express it.
- Touch targets at least 44×44 px on touch devices; layouts work from 320 px wide and at 200% zoom; respect `prefers-reduced-motion`.

## Usability (should)
- One primary action per screen, labelled with a verb ("Print label", not "OK").
- Validate on blur and on submit, keep what the user typed, and put the error next to the field.
- Loading: nothing under 100 ms, inline indicator up to 1 s, skeleton beyond; never block the whole screen for a background save.
- Empty states say what goes here and offer the action that fills it.
- Destructive actions name what will be destroyed and offer undo where possible.
- Operational screens (warehouse, courier, back office): large targets, scanner-friendly inputs, works offline or degrades clearly, minimal typing.
