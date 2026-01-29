# VHS Demo GIFs Design

## Goal

Create demo GIFs using Charm's VHS to make the README more engaging and show guppi's value at a glance.

## GIFs to Create

### 1. Hero GIF (`demo.gif`)

**Purpose:** Show the main value proposition - updating multiple repos with one keypress.

**Specs:**
- Size: 800x500
- Duration: ~8 seconds
- Location in README: Right after the selling point quote

**Flow:**
1. Type `guppi` → Enter
2. Show list with mixed statuses (some ✓ synced, some ↓ behind)
3. Press `A` to pull all behind
4. Watch the ↓ repos update to ✓ (synced ones stay unchanged)
5. Brief pause, `q` to quit

### 2. Detail View GIF (`detail-view.gif`)

**Purpose:** Demonstrate the multi-pane detail view for deeper repo inspection.

**Specs:**
- Size: 800x500
- Duration: ~6 seconds
- Location in README: Detail View section under Key Bindings

**Flow:**
1. Start in list view (guppi already running)
2. Press `d` to open detail view
3. Show uncommitted changes in the status pane (left)
4. Press `Tab` to switch between panes
5. Brief pause on branches pane showing indicators (↕ ⚠ ☁)
6. Press `Esc` to go back

## File Structure

```
tapes/
  demo.tape
  detail-view.tape
assets/
  demo.gif
  detail-view.gif
```

## README Changes

1. Add selling point quote after badges:
   > **Stop running `git pull` in 20 different terminals.**
   >
   > guppi gives you a single view of all your repositories - see what's changed, what's behind, and pull updates in seconds. Perfect for the morning "sync everything" routine.

2. Add `![guppi demo](assets/demo.gif)` after selling point

3. Add `![detail view](assets/detail-view.gif)` in Detail View section
