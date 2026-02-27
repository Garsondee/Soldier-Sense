# Design Decision Records

This folder contains the design decision records (DDRs) for the Soldier-Sense project. Each record documents a significant design or architecture choice — what the issue was, what options were considered, and what was decided and why.

## Structure

Each decision lives in its own numbered subfolder:

```
docs/decisions/
  0000-some-topic/
    0000-some-topic.md
  0001-another-topic/
    0001-another-topic.md
  ...
  decision-template.md   ← template used when creating new records
```

The four-digit prefix provides a stable chronological ordering. Additional supporting files (diagrams, references, etc.) can be placed alongside the `.md` file in the same subfolder.

## Record format

Every decision record follows the template in [decision-template.md](decision-template.md) and contains these sections:

| Section | Purpose |
|---|---|
| **Issue** | The problem or question that required a decision |
| **Factors** | Constraints and concerns that shaped the decision |
| **Options and Outcome** | The alternatives considered and which one was chosen |
| **Consequences** | The impact and implications of the decision |
| **Pros and Cons of Options** | Detailed trade-off notes for each option |
| **Additional Notes** | Any supplementary context |

## Managing records

Use `scripts/decisions.py` (or the `just` recipes that wrap it) rather than creating files by hand, so that numbering stays consistent.

### Initialise (first-time setup)

```bash
python3 scripts/decisions.py --init
# or
just init-decisions
```

Creates the `docs/decisions` directory and writes `decision-template.md` if they do not already exist.

### Add a new decision record

```bash
python3 scripts/decisions.py --add "Your Decision Topic"
# or
just add-decision "Your Decision Topic"
```

This will:

1. Determine the next available four-digit serial number by scanning existing subfolders.
2. Create `docs/decisions/<NNNN>-your-decision-topic/<NNNN>-your-decision-topic.md`.
3. Populate the file from `decision-template.md`, substituting:
   - `{{Decision Title}}` → the topic you provided
   - `{{YYYY-MM-DD}}` → today's date
   - `{{RecordID}}` → the zero-padded serial number

After generation, open the file and fill in the sections with the actual decision content.
