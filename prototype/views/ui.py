"""Shared look for every page: pill tabs, step headers, pills. One accent, readable in dark and light."""

import html

import streamlit as st

ACCENT = "#2E5E8C"


def apply_theme() -> None:
    st.html(f"""
<style>
  /* Streamlit 1.64 tabs (React Aria): role=tablist, data-testid=stTab, a separate underline indicator */
  [role="tablist"] {{ gap: 8px; flex-wrap: wrap; border-bottom: none !important; box-shadow: none !important;
                      padding-bottom: 6px; }}
  [role="tablist"]::after, [role="tablist"]::before {{ display: none !important; }}
  .react-aria-SelectionIndicator {{ display: none !important; }}
  [data-testid="stTab"] {{
    border: 1px solid rgba(128,128,128,.35) !important; border-radius: 999px !important; padding: 6px 16px !important;
    background: rgba(128,128,128,.06); margin: 0 !important;
  }}
  [data-testid="stTab"]:hover {{ border-color: {ACCENT} !important; }}
  [data-testid="stTab"][aria-selected="true"] {{ background: {ACCENT} !important; border-color: {ACCENT} !important; }}
  [data-testid="stTab"][aria-selected="true"] p, [data-testid="stTab"][aria-selected="true"] span {{ color: #fff !important; }}
  [data-testid="stTab"] p {{ font-size: 0.95rem; font-weight: 600; }}
  .flow {{ display: flex; flex-wrap: wrap; align-items: stretch; gap: 6px; margin: 4px 0 14px; }}
  .flow .step {{ flex: 1 1 120px; border: 1px solid rgba(128,128,128,.3); border-radius: 10px; padding: 8px 10px;
                 background: rgba(128,128,128,.05); }}
  .flow .step .k {{ font-size: .72rem; text-transform: uppercase; letter-spacing: .04em; opacity: .7; }}
  .flow .step .v {{ font-size: 1.25rem; font-weight: 700; line-height: 1.3; }}
  .flow .step .s {{ font-size: .75rem; opacity: .75; }}
  .flow .arrow {{ align-self: center; opacity: .45; font-size: 1.1rem; }}
  .stephead {{ border-left: 4px solid {ACCENT}; background: rgba(46,94,140,.08); border-radius: 6px;
               padding: 10px 14px; margin: 6px 0 14px; }}
  .stephead .t {{ font-size: 1.15rem; font-weight: 700; margin-bottom: 2px; }}
  .stephead .io {{ font-size: .85rem; opacity: .8; margin-top: 4px; }}
  .pill {{ display: inline-block; padding: 2px 11px; margin: 0 5px 5px 0; border-radius: 999px; font-size: .8rem;
           font-weight: 600; border: 1px solid; white-space: nowrap; }}
  /* mid-tone colors: readable on the dark and the light theme alike */
  .pill.good   {{ color: #4caf50; border-color: rgba(76,175,80,.55);  background: rgba(76,175,80,.14); }}
  .pill.warn   {{ color: #e08a2e; border-color: rgba(224,138,46,.55); background: rgba(224,138,46,.14); }}
  .pill.bad    {{ color: #e05252; border-color: rgba(224,82,82,.55);  background: rgba(224,82,82,.12); }}
  .pill.info   {{ color: #5b93cf; border-color: rgba(91,147,207,.6);  background: rgba(91,147,207,.14); }}
  .pill.violet {{ color: #a974cc; border-color: rgba(169,116,204,.55); background: rgba(169,116,204,.14); }}
  .pill.muted  {{ color: inherit; opacity: .8; border-color: rgba(128,128,128,.45); background: rgba(128,128,128,.08); }}
  .row {{ border: 1px solid rgba(128,128,128,.25); border-radius: 10px; padding: 8px 12px; margin-bottom: 8px; }}
  .row .names {{ font-weight: 600; margin-bottom: 4px; }}
  .row .why {{ font-size: .85rem; opacity: .8; margin-top: 2px; }}
  .note {{ border: 1px solid rgba(91,147,207,.45); background: rgba(91,147,207,.08); border-radius: 8px;
           padding: 10px 14px; font-size: .9rem; line-height: 1.5; margin-bottom: 12px; }}
  .rcard {{ border: 1px solid rgba(128,128,128,.3); border-radius: 12px; padding: 12px 16px; margin-bottom: 12px; }}
  .fcard {{ font-size: .82rem; padding: 10px 12px; }}
  .fcard .names {{ font-size: .95rem; margin: 6px 0 2px; }}
  .fcard .pill {{ font-size: .72rem; padding: 1px 9px; }}
  .rcard .names {{ font-weight: 700; font-size: 1.02rem; margin-bottom: 6px; }}
  .trace {{ list-style: none; padding: 0; margin: 8px 0 0; }}
  .trace li {{ display: flex; gap: 10px; padding: 6px 0; border-top: 1px dashed rgba(128,128,128,.25); }}
  .trace .ic {{ flex: 0 0 22px; height: 22px; border-radius: 50%; text-align: center; line-height: 22px; font-weight: 700;
                font-size: .8rem; }}
  .trace li.ok .ic {{ background: rgba(76,175,80,.18); color: #4caf50; }}
  .trace li.fail .ic {{ background: rgba(224,82,82,.18); color: #e05252; }}
  .trace li.skip {{ opacity: .5; }}
  .trace li.skip .ic {{ background: rgba(128,128,128,.15); }}
  .bar {{ display: flex; align-items: center; gap: 8px; margin: 4px 0; font-size: .85rem; }}
  .bar .lbl {{ flex: 0 0 90px; }} .bar .amt {{ flex: 0 0 60px; font-weight: 700; }}
  .bar .track {{ flex: 1 1 auto; max-width: 420px; height: 10px; border-radius: 999px; background: rgba(128,128,128,.15); }}
  .bar .fill {{ display: block; height: 100%; border-radius: 999px; background: rgba(128,128,128,.55); }}
  .bar .fill.win {{ background: #4caf50; }}
  .hero .lede {{ font-size: 1.12rem; line-height: 1.5; margin: -6px 0 12px; max-width: 980px; }}
  .hero .beats {{ display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 10px; margin-bottom: 16px; }}
  .hero .beat {{ border-top: 3px solid {ACCENT}; background: rgba(128,128,128,.06); border-radius: 8px;
                 padding: 10px 12px; font-size: .88rem; line-height: 1.45; }}
  .hero .beat .h {{ font-weight: 700; font-size: .95rem; margin-bottom: 4px; }}
</style>
""")


def step_header(title: str, what: str, inputs: str, outputs: str) -> None:
    st.html(f'<div class="stephead"><div class="t">{html.escape(title)}</div><div>{html.escape(what)}</div>'
            f'<div class="io"><b>In:</b> {html.escape(inputs)} &nbsp;&nbsp; <b>Out:</b> {html.escape(outputs)}</div></div>')


def pill(text, kind="muted") -> str:
    return f'<span class="pill {kind}">{html.escape(str(text))}</span>'


TIER = {"strong": "good", "near_miss": "warn", "dropped": "bad"}
RELATION = {"same": "info", "opposite": "violet"}
CHECK = {"unchanged": "good", "new_version": "warn", "fetch_failed": "bad", "failed": "bad"}


def decision_pill(d, stale) -> str:
    if d is None:
        return pill("to review", "muted")
    if stale:
        return pill(f"{d['verdict']} · stale", "warn")
    return pill(f"approved {d['verdict']}", "good") if d["verdict"] != "reject" else pill("rejected", "bad")


def pattern_pills(p) -> str:
    return "".join(pill(x, "warn") for x in sorted(p.soft)) + "".join(pill(x, "bad") for x in sorted(p.hard))


def action_pill(action) -> str:
    return pill(action.replace("_", " "), "bad" if action == "no_route" else "good")
