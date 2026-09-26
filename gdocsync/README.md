# gdocsync

Sync Google Docs from Google Drive into Markdown.

`gdocsync` is a small command-line tool that:

1. Downloads Google Docs from a Drive folder as `.docx` files (read-only access).
2. Converts those `.docx` files into Markdown using [Pandoc](https://pandoc.org/).

It is designed for one-way sync: **Google Drive → your local machine**. It never modifies or uploads anything back to Drive.

---

## How it works

```
Google Drive folder
        │
        ▼
   gdocsync sync      →   .docx files
        │
        ▼
   gdocsync convert   →   .md files
```

The `sync` step talks to Google Drive. The `convert` step is fully offline and just runs Pandoc locally.

---

## Requirements

- **Go** (for building) — this project uses Go modules.
- **Pandoc** — required only for the `convert` command.
  - macOS: `brew install pandoc`
  - Linux: `sudo apt install pandoc`
  - Windows: [download here](https://pandoc.org/installing.html)

Check it's installed:

```bash
pandoc --version
```

---

## Build

```bash
git clone <your-repo-url> gdocsync
cd gdocsync
go build -o gdocsync .
```

This produces a single `gdocsync` binary.

---

## Setup (one-time)

### 1. Create Google OAuth credentials

1. Go to the [Google Cloud Console](https://console.cloud.google.com/).
2. Create a project (or use an existing one).
3. Enable the **Google Drive API**.
4. Go to **APIs & Services → Credentials**.
5. Click **Create Credentials → OAuth client ID**.
6. Choose **Desktop app**.
7. Download the JSON file.
8. Rename it to `credentials.json`.

### 2. Place `credentials.json` next to the binary

`gdocsync` looks for `credentials.json`, `token.json`, and `gdocsync-state.json` in the **directory of the executable**.

```
your-folder/
├── gdocsync          ← the binary
└── credentials.json  ← put it here
```

> If you run `go run .` during development, it falls back to the current working directory.

### 3. Authenticate

```bash
./gdocsync auth
```

A browser window opens. Sign in and grant read-only Drive access. A `token.json` is saved next to the binary.

You only need to do this once. To revoke it later, run `./gdocsync logout`.

---

## Usage

```bash
gdocsync <command> [options]
```

### Commands

| Command   | What it does                                              |
|-----------|-----------------------------------------------------------|
| `auth`    | Authenticate with Google Drive                            |
| `list`    | List Google Docs inside a Drive folder                    |
| `sync`    | Download Google Docs as `.docx` files                     |
| `convert` | Convert `.docx` files to Markdown (uses Pandoc, no Drive) |
| `logout`  | Remove the local OAuth token                              |
| `version` | Show version                                              |

---

### `list` — see what's in a folder

```bash
gdocsync list <folder-url-or-id>
```

You can paste either a Drive folder URL:

```bash
gdocsync list "https://drive.google.com/drive/folders/ABC123"
```

…or just the folder ID:

```bash
gdocsync list ABC123
```

---

### `sync` — download as `.docx`

```bash
gdocsync sync <folder-url-or-id> [options]
```

**Options:**

| Flag              | Default    | Description                            |
|-------------------|------------|----------------------------------------|
| `--output <dir>`  | `output`   | Where to write the `.docx` files       |
| `--force`         | `false`    | Re-download everything                 |
| `--dry-run`       | `false`    | Show what would happen, write nothing  |

**Examples:**

```bash
# Download everything into ./output
gdocsync sync ABC123

# Download into ./chapters
gdocsync sync ABC123 --output chapters

# Preview only
gdocsync sync ABC123 --dry-run

# Force re-download
gdocsync sync ABC123 --force
```

Downloaded files are tracked in `gdocsync-state.json`. On the next sync, unchanged docs are skipped automatically.

---

### `convert` — turn `.docx` into Markdown

```bash
gdocsync convert <input-dir> [options]
```

**Options:**

| Flag              | Default        | Description                         |
|-------------------|----------------|-------------------------------------|
| `--output <dir>`  | input dir      | Where to write `.md` files          |
| `--force`         | `false`        | Reconvert everything                |

**Examples:**

```bash
# Convert in place
gdocsync convert chapters

# Convert into a different folder
gdocsync convert chapters --output markdown
```

---

## Full workflow example

```bash
# 1. One-time auth
./gdocsync auth

# 2. Look at what's in the folder
./gdocsync list "https://drive.google.com/drive/folders/ABC123"

# 3. Download all docs as .docx
./gdocsync sync ABC123 --output chapters

# 4. Convert them to Markdown
./gdocsync convert chapters --output markdown
```

Result:

```
chapters/           markdown/
├── 01-intro.docx   ├── 01-intro.md
├── 02-setup.docx   ├── 02-setup.md
└── 03-usage.docx   └── 03-usage.md
```

---

## Notes

- **Read-only.** The app only requests `drive.readonly`. It cannot modify your Drive.
- **Flags can go anywhere.** `gdocsync convert chapters --output md` and `gdocsync convert --output md chapters` both work.
- **Filenames** are slugified (lowercased, spaces → dashes, numbers preserved).
- **Ordering** is numeric-aware: `Chapter 2` sorts before `Chapter 10`.
- **State file** (`gdocsync-state.json`) is used by `sync` to skip unchanged files.
- **Interrupt safely.** `Ctrl+C` during a sync stops cleanly.

---

## Files created by gdocsync

| File                     | Purpose                                  |
|--------------------------|------------------------------------------|
| `credentials.json`       | You provide this (Google OAuth client)   |
| `token.json`             | Created by `auth`, removed by `logout`   |
| `gdocsync-state.json`    | Tracks downloaded docs to skip unchanged ones |

All three live next to the binary.
