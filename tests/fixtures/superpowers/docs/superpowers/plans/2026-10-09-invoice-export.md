# Invoice Export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Finance can export invoices to CSV.

## Global Constraints

- Money as decimal strings

### Task 1: CSV writer

**Files:**
- Create: `src/export/csv.ts`
- Test: `tests/export/csv.test.ts`

- [x] **Step 1: Write the failing test**
- [x] **Step 2: Run test to verify it fails**
- [x] **Step 3: Implement `toCsv(rows: Invoice[]) -> string` in `src/export/csv.ts`**
- [x] **Step 4: Run test to verify it passes**

### Task 2: Export endpoint

**Files:**
- Modify: `src/routes/invoices.ts`

- [ ] **Step 1: Write the failing test for GET /invoices.csv**
- [ ] **Step 2: Implement the route**

## Self-Review
