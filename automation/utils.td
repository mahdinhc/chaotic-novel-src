import "strings"

// Splits a chapter file's text into {date, title, body} using simple line checks.
fn parse_chapter(text) {
    lines := strings.split(text, "\n")

    date := null
    title := null
    bodyLines := []

    for line in lines {
        trimmed := strings.trim_space(line)

        // Skip empty lines at the top, but keep them once body starts
        if len(bodyLines) == 0 && len(trimmed) == 0 {
            continue
        }
		
        // First non-empty line that looks like *[...]* is the date
		or1 := strings.has_prefix(trimmed, "*[") || strings.has_prefix(trimmed, "*\\[")
		or2 := strings.has_suffix(trimmed, "]*") || strings.has_suffix(trimmed, "\\]*")
        if date == null && or1 && or2 {
            date = strings.substr(trimmed, 2, len(trimmed) - 3)
            continue
        }

        // Next non-empty line that starts with "Chapter " is the title
		or := strings.has_prefix(trimmed, "Chapter ") || strings.has_prefix(trimmed, "*Chapter ")
        if title == null && or {
            title = trimmed
            continue
        }

        bodyLines = append(bodyLines, line)
    }

    body := strings.trim_space(strings.join(bodyLines, "\n"))

    return { "date": date, "title": title, "body": body }
}

fn slug(text) {
    s := strings.to_lower(text)
    out := ""
    lastWasDash := false

    for ch in s {
        c := string(ch)
        if (c >= "a" && c <= "z") || (c >= "0" && c <= "9") {
            out = out + c
            lastWasDash = false
        } else {
            if !lastWasDash && len(out) > 0 {
                out = out + "-"
                lastWasDash = true
            }
        }
    }

    // trim trailing dash
    if strings.has_suffix(out, "-") {
        out = strings.substr(out, 0, len(out) - 1)
    }

    return out
}

fn parse_idx(s) {
    // try "Chapter N" first
    m := strings.re_find(`Chapter\s+([0-9]+(?:\.[0-9]+)?)`, s, -1)
    if m != null && len(m) > 0 {
        return m[0][1].strings
    }

    // fall back to "N.md" filename style
    m = strings.re_find(`([0-9]+(?:\.[0-9]+)?)\.md$`, s, -1)
    if m != null && len(m) > 0 {
        return m[0][1].strings
    }

    return "?"
}

export {
	parse_chapter: parse_chapter,
    slug: slug,
    parse_idx: parse_idx
}