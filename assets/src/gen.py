#!/usr/bin/env python3
"""Generate source SVGs (with <text>) for the gh-envy README banner and nav buttons."""
import os

OUT = os.environ.get("OUT") or os.path.dirname(os.path.abspath(__file__))
FONT = "JetBrainsMono Nerd Font"

THEMES = {
    "dark": dict(bg="#1a1b26", accent="#9ece6a", secondary="#7aa2f7", muted="#a9b1d6"),
    "light": dict(bg="#f5f7f0", accent="#4f7a1f", secondary="#2e5cb8", muted="#4c505e"),
}


def banner(t):
    W, H = 1280, 320
    a, s, m, bg = t["accent"], t["secondary"], t["muted"], t["bg"]
    # left text block
    x0 = 88
    wm_size = 100
    adv = 0.6 * wm_size
    wm_base = 190
    cursor_x = x0 + adv * 7 + 14
    parts = []
    parts.append(f'<rect x="0.75" y="0.75" width="{W-1.5}" height="{H-1.5}" rx="20" fill="{bg}" stroke="{m}" stroke-opacity="0.18" stroke-width="1.5"/>')
    # prompt line
    parts.append(
        f'<text x="{x0}" y="{wm_base-104}" font-family="{FONT}" font-size="22" font-weight="400" fill="{m}" fill-opacity="0.75">'
        f'<tspan fill="{a}" fill-opacity="1">$</tspan> gh envy approve -w deploy.yml</text>'
    )
    parts.append(
        f'<text x="{x0}" y="{wm_base}" font-family="{FONT}" font-size="{wm_size}" font-weight="700" fill="{m}">'
        f'gh <tspan fill="{a}">envy</tspan></text>'
    )
    parts.append(f'<rect x="{cursor_x:.1f}" y="{wm_base-72}" width="{adv*0.42:.1f}" height="72" rx="3" fill="{a}" fill-opacity="0.45"/>')
    parts.append(
        f'<text x="{x0+4}" y="{wm_base+62}" font-family="{FONT}" font-size="22" font-weight="400" fill="{m}">'
        f'Deployment environments for the GitHub CLI</text>'
    )

    # pipeline on the right
    bw, bh, gap = 150, 64, 32
    px = W - 88 - (3 * bw + 2 * gap)
    py = 126
    stages = [
        ("dev", "branch: *", s, False),
        ("staging", "branch: main", s, False),
        ("production", "tag: v*", a, True),
    ]
    for i, (name, rule, col, gated) in enumerate(stages):
        bx = px + i * (bw + gap)
        parts.append(
            f'<rect x="{bx}" y="{py}" width="{bw}" height="{bh}" rx="12" fill="{col}" fill-opacity="{0.14 if gated else 0.08}" '
            f'stroke="{col}" stroke-width="{2.5 if gated else 2}"/>'
        )
        parts.append(
            f'<text x="{bx+bw/2}" y="{py+bh/2+7}" text-anchor="middle" font-family="{FONT}" font-size="19" font-weight="700" fill="{col}">{name}</text>'
        )
        parts.append(
            f'<text x="{bx+bw/2}" y="{py+bh+30}" text-anchor="middle" font-family="{FONT}" font-size="15" font-weight="400" fill="{m}" fill-opacity="0.85">{rule}</text>'
        )
        if i < 2:
            ax0 = bx + bw + 7
            ax1 = bx + bw + gap - 7
            ay = py + bh / 2
            parts.append(f'<path d="M{ax0} {ay}H{ax1-2}" stroke="{m}" stroke-opacity="0.6" stroke-width="2" stroke-linecap="round"/>')
            parts.append(f'<path d="M{ax1-8} {ay-6}L{ax1} {ay}L{ax1-8} {ay+6}" fill="none" stroke="{m}" stroke-opacity="0.6" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>')
        if gated:
            # shield badge centred on the top edge of the production box
            cx, cy = bx + bw / 2, py - 10
            sw, sh = 30, 36
            top = cy - sh / 2
            shield = (
                f"M{cx} {top}"
                f"L{cx+sw/2} {top+6}"
                f"V{top+sh*0.45}"
                f"C{cx+sw/2} {top+sh*0.75} {cx+sw*0.22} {top+sh*0.92} {cx} {top+sh}"
                f"C{cx-sw*0.22} {top+sh*0.92} {cx-sw/2} {top+sh*0.75} {cx-sw/2} {top+sh*0.45}"
                f"V{top+6}Z"
            )
            parts.append(f'<path d="{shield}" fill="{bg}" stroke="{a}" stroke-width="2.5" stroke-linejoin="round"/>')
            parts.append(f'<path d="{shield}" fill="{a}" fill-opacity="0.18"/>')
            parts.append(
                f'<path d="M{cx-7} {top+sh*0.47}L{cx-1.5} {top+sh*0.62}L{cx+8} {top+sh*0.32}" fill="none" stroke="{a}" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"/>'
            )
            # reviewers + wait timer note under the rule
    # gate caption spanning the pipeline
    parts.append(
        f'<text x="{px + 2*(bw+gap) + bw/2}" y="{py+bh+54}" text-anchor="middle" font-family="{FONT}" font-size="15" font-weight="400" fill="{a}">2 reviewers · wait 5m</text>'
    )
    body = "\n  ".join(parts)
    return f'<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{H}" viewBox="0 0 {W} {H}">\n  {body}\n</svg>\n'


NAV = [
    ("install", "Install", ""),
    ("quickstart", "Quickstart", ""),
    ("commands", "Commands", ""),
    ("docs", "Docs", ""),
]


def button(t, label, icon):
    margin = 6
    bw, bh = 164, 40
    W = bw + 2 * margin
    a = t["accent"]
    fs = 16
    icon_w = 16
    gapi = 9
    text_w = 0.6 * fs * len(label)
    content = icon_w + gapi + text_w
    x = margin + (bw - content) / 2
    base = bh / 2 + fs * 0.365
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{bh}" viewBox="0 0 {W} {bh}">\n'
        f'  <rect x="{margin+0.75}" y="0.75" width="{bw-1.5}" height="{bh-1.5}" rx="{(bh-1.5)/2}" fill="{a}" fill-opacity="0.12" stroke="{a}" stroke-width="1.5"/>\n'
        f'  <text x="{x:.2f}" y="{base:.2f}" font-family="{FONT}" font-size="{fs}" font-weight="400" fill="{a}">{icon}</text>\n'
        f'  <text x="{x+icon_w+gapi:.2f}" y="{base:.2f}" font-family="{FONT}" font-size="{fs}" font-weight="700" fill="{a}">{label}</text>\n'
        f'</svg>\n'
    )


for name, t in THEMES.items():
    with open(os.path.join(OUT, f"src-banner-{name}.svg"), "w") as f:
        f.write(banner(t))
    for slug, label, icon in NAV:
        with open(os.path.join(OUT, f"src-{slug}-{name}.svg"), "w") as f:
            f.write(button(t, label, icon))
