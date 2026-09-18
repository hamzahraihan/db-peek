# Query Buffers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Multi-buffer query tabs so a new query doesn't require erasing the current one; modal vim switch via Esc then H/L.

**Architecture:** Keep active `editor/querySample/.../queryTable` fields as the working copy; add `qbufs []queryBuffer` + `qcur` with save/load on switch/new/close. Async `queryDoneMsg` carries buffer ID + seq so late replies paint the right buffer. Strip reuses the blank row after tabs so mouse Y math is unchanged.

**Tech Stack:** Go, Bubble Tea, existing TUI MVU.

## Global Constraints

- No Alt bindings (GlazeWM owns Alt).
- Plain H/L only in results mode (queryFocus==1); typing mode must keep inserting runes.
- Editor stays 8 rows; queryResultsTop unchanged.
- Cap 5 buffers v1, no persistence, no mouse on strip.
---

### Task 1: Buffer state + helpers

**Files:**
- Create: `internal/tui/querybuf.go`
- Modify: `internal/tui/model.go`
- Test: `internal/tui/querybuf_test.go`

**Interfaces:**
- Consumes: `Editor`, `dbpkg.Sample`, `dataTable`
- Produces: `type queryBuffer`, `(m *Model) ensureQueryBufs/saveActiveBuf/loadActiveBuf/newQueryBuf/closeQueryBuf/switchQueryBuf`, `(m Model) queryStripView`, `queryBufTitle`

- [ ] **Step 1: Write failing test** for ensure/new/switch preserving editor text.
- [ ] **Step 2: Run test, verify fail.**
- [ ] **Step 3: Implement querybuf.go + model fields + New() init.**
- [ ] **Step 4: Run test, verify pass.**
- [ ] **Step 5: Commit.**

### Task 2: Async routing per buffer

**Files:**
- Modify: `internal/tui/msg.go`, `internal/tui/commands.go`, `internal/tui/update.go`
- Test: `internal/tui/querybuf_test.go`

**Interfaces:**
- Consumes: `queryBuffer.seq/id`, `Model.qbufs/qcur/querySeq`
- Produces: `queryDoneMsg.qbufID`, `runQuery` capturing active buf, `applyQueryDoneToBuf` semantics.

- [ ] **Step 1: Write failing test** for stale/foreign buffer reply isolation.
- [ ] **Step 2-5: Implement + test + commit.**

### Task 3: Keys H/L/t/X + ctrl+t/w

**Files:**
- Modify: `internal/tui/update.go`
- Test: `internal/tui/querybuf_test.go`

**Interfaces:**
- Consumes: switch/new/close helpers, `clampEditorScroll`, `refreshCompletion`, `sizeTables`.
- Produces: key handling in editor + results branches.

- [ ] **Steps: test-first for H/L in results, t/X, ctrl+t/w in both; implement; commit.**

### Task 4: Strip render + help

**Files:**
- Modify: `internal/tui/view_detail.go`, `internal/tui/whichkey.go`
- Test: `internal/tui/querybuf_test.go`, `internal/tui/view_fit_test.go`

- [ ] **Steps: strip replaces blank row for tab==3; hint update; which-key; budget test; commit.**

### Task 5: Full regression

- [ ] **Run `go test ./...`, `go vet ./...`, verify.**
