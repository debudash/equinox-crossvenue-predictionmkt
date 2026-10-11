"""Equinox, one place: the core (live feed, match and route results) and the proposal prototype.
streamlit run app.py  (from prototype/)"""

import hmac
import os

import streamlit as st

from views.ui import apply_theme

st.set_page_config(page_title="Equinox", page_icon=":material/compare_arrows:", layout="wide")
apply_theme()

# Hosted deployments set EQX_APP_PASSCODE: the whole app sits behind a sign-in. Unset locally, so no gate.
# The username is fixed for now (one shared demo account); the password comes from the environment.
USERNAME = os.getenv("EQX_APP_USERNAME", "peak6")
PASSCODE = os.getenv("EQX_APP_PASSCODE", "")


def _same(a: str, b: str) -> bool:
    return hmac.compare_digest(a.strip().encode(), b.encode())   # stray spaces from a paste are ignored


if PASSCODE and not st.session_state.get("unlocked"):
    _, mid, _ = st.columns([1, 1.2, 1])
    with mid:
        st.html("""<div style="margin-top:12vh; text-align:center">
          <div style="font-size:2.1rem; font-weight:800; letter-spacing:-.01em">Equinox</div>
          <div style="opacity:.75; margin:4px 0 22px">Cross-venue equivalence and routing · Kalshi × Polymarket</div>
        </div>""")
        with st.form("sign-in", border=True):
            user = st.text_input("Username", autocomplete="username")
            code = st.text_input("Password", type="password", autocomplete="current-password")
            if st.form_submit_button("Sign in", type="primary", icon=":material/login:", width="stretch"):
                if _same(user.lower(), USERNAME.lower()) and _same(code, PASSCODE):
                    st.session_state["unlocked"] = True
                    st.rerun()
                st.error("Username or password is not right.")
        st.caption("Private demo. Ask the Equinox team for access.")
    st.stop()

with st.sidebar:
    if PASSCODE and st.button("Sign out", icon=":material/logout:", type="tertiary"):
        st.session_state["unlocked"] = False
        st.rerun()

nav = st.navigation({
    "Equinox core": [
        st.Page("views/core_results.py", title="Match & route results", icon=":material/fact_check:", default=True),
        st.Page("views/core_feed.py", title="Live feed", icon=":material/dynamic_feed:"),
    ],
    "Proposal prototype": [
        st.Page("views/guided.py", title="Guided test", icon=":material/route:"),
        st.Page("views/proposal.py", title="Near-miss proposal", icon=":material/compare_arrows:"),
    ],
})
if nav.title not in ("Near-miss proposal", "Guided test"):
    with st.sidebar:
        st.divider()
        st.segmented_control("Core data", ["Live", "Synthetic demo"], key="core_source", default="Live",
                             help="Live: the markets Equinox tracks now. Synthetic demo: fixture markets built to match, "
                                  "so a match and its automatic route can be shown.")
nav.run()
