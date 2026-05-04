use napi_derive::napi;
use serde::{Deserialize, Serialize};
use syntect::easy::HighlightLines;
use syntect::highlighting::ThemeSet;
use syntect::parsing::SyntaxSet;
use syntect::util::{as_24_bit_terminal_escaped, LinesWithEndings};
use unicode_width::UnicodeWidthChar;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct RenderInput {
    file_path: String,
    lines: Vec<DiffLine>,
    width: usize,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct DiffLine {
    kind: String,
    old_line: Option<usize>,
    new_line: Option<usize>,
    text: String,
}

#[derive(Debug, Serialize)]
struct RenderedLine {
    gutter: String,
    content: String,
}

const RESET: &str = "\x1b[0m";
const FG_DIM: &str = "\x1b[38;5;244m";
const FG_ADD: &str = "\x1b[38;5;114m";
const FG_DEL: &str = "\x1b[38;5;203m";
const BG_ADD: &str = "\x1b[48;5;22m";
const BG_DEL: &str = "\x1b[48;5;52m";

#[napi(js_name = "renderColorDiffJson")]
pub fn render_color_diff_json(input: String) -> napi::Result<String> {
    let input: RenderInput = serde_json::from_str(&input)
        .map_err(|err| napi::Error::from_reason(format!("invalid color diff input: {err}")))?;
    let rows = render_rows(&input);
    serde_json::to_string(&rows)
        .map_err(|err| napi::Error::from_reason(format!("failed to encode color diff output: {err}")))
}

fn render_rows(input: &RenderInput) -> Vec<RenderedLine> {
    let ps = SyntaxSet::load_defaults_newlines();
    let ts = ThemeSet::load_defaults();
    let theme = ts
        .themes
        .get("base16-ocean.dark")
        .or_else(|| ts.themes.values().next());
    let syntax = ps
        .find_syntax_for_file(&input.file_path)
        .ok()
        .flatten()
        .unwrap_or_else(|| ps.find_syntax_plain_text());
    let gutter_width = gutter_width(&input.lines);
    let content_width = input.width.saturating_sub(gutter_width + 1).max(8);

    input
        .lines
        .iter()
        .map(|line| {
            let marker = marker(&line.kind);
            let line_no = if line.kind == "added" {
                line.new_line
            } else {
                line.old_line.or(line.new_line)
            };
            let gutter = format!(
                "{}{} {:>width$}{}",
                marker_color(&line.kind),
                marker,
                line_no.map(|n| n.to_string()).unwrap_or_default(),
                RESET,
                width = gutter_width.saturating_sub(2)
            );
            let content = if let Some(theme) = theme {
                highlighted_line(&ps, theme, syntax, &line.text, &line.kind)
            } else {
                fallback_line(&line.text, &line.kind)
            };
            RenderedLine {
                gutter,
                content: format!("{}{}{}", line_bg(&line.kind), truncate_plain(&content, content_width), RESET),
            }
        })
        .collect()
}

fn marker(kind: &str) -> &'static str {
    match kind {
        "added" => "+",
        "removed" => "-",
        _ => " ",
    }
}

fn marker_color(kind: &str) -> &'static str {
    match kind {
        "added" => FG_ADD,
        "removed" => FG_DEL,
        _ => FG_DIM,
    }
}

fn line_bg(kind: &str) -> &'static str {
    match kind {
        "added" => BG_ADD,
        "removed" => BG_DEL,
        _ => "",
    }
}

fn gutter_width(lines: &[DiffLine]) -> usize {
    let max_line = lines
        .iter()
        .flat_map(|line| [line.old_line, line.new_line])
        .flatten()
        .max()
        .unwrap_or(1);
    max_line.to_string().len() + 2
}

fn highlighted_line(
    ps: &SyntaxSet,
    theme: &syntect::highlighting::Theme,
    syntax: &syntect::parsing::SyntaxReference,
    text: &str,
    kind: &str,
) -> String {
    let mut highlighter = HighlightLines::new(syntax, theme);
    let mut out = String::new();
    for line in LinesWithEndings::from(text) {
        match highlighter.highlight_line(line, ps) {
            Ok(ranges) => out.push_str(&as_24_bit_terminal_escaped(&ranges[..], false)),
            Err(_) => out.push_str(&fallback_line(text, kind)),
        }
    }
    if out.is_empty() {
        fallback_line(text, kind)
    } else {
        out
    }
}

fn fallback_line(text: &str, kind: &str) -> String {
    let fg = match kind {
        "added" => FG_ADD,
        "removed" => FG_DEL,
        _ => "\x1b[38;5;250m",
    };
    format!("{fg}{text}")
}

fn truncate_plain(input: &str, max_width: usize) -> String {
    if max_width == 0 {
        return String::new();
    }
    if display_width(input) <= max_width {
        return input.to_string();
    }
    let ellipsis = '…';
    let ellipsis_width = UnicodeWidthChar::width(ellipsis).unwrap_or(1);
    let limit = max_width.saturating_sub(ellipsis_width);
    let mut out = String::new();
    let mut visible = 0usize;
    let mut chars = input.chars().peekable();
    while let Some(ch) = chars.next() {
        if ch == '\x1b' {
            out.push(ch);
            for next in chars.by_ref() {
                out.push(next);
                if next == 'm' {
                    break;
                }
            }
            continue;
        }
        let width = UnicodeWidthChar::width(ch).unwrap_or(0);
        if visible + width > limit {
            break;
        }
        out.push(ch);
        visible += width;
    }
    out.push(ellipsis);
    out
}

fn display_width(input: &str) -> usize {
    let mut width = 0usize;
    let mut chars = input.chars().peekable();
    while let Some(ch) = chars.next() {
        if ch == '\x1b' {
            for next in chars.by_ref() {
                if next == 'm' {
                    break;
                }
            }
            continue;
        }
        width += UnicodeWidthChar::width(ch).unwrap_or(0);
    }
    width
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn truncate_plain_limits_by_display_width_for_cjk() {
        let input = "\x1b[38;5;203m成功使用file_write工具写入此行内容zheli shi ceshi wenben";
        let out = truncate_plain(input, 18);
        assert!(display_width(&out) <= 18);
        assert!(out.ends_with('…'));
    }

    #[test]
    fn truncate_plain_keeps_short_line() {
        let input = "\x1b[38;5;114mshort line";
        let out = truncate_plain(input, 40);
        assert_eq!(out, input);
    }
}
