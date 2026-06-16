#!/usr/bin/env python3
"""Format a banish release as a Discord embed and post it to a webhook.

This is a pure formatter over the Git release notes -- the single source of
truth. It reads its inputs from the environment, builds the embed per the
release spec (number first, contributors named, mascot thumbnail), and POSTs it.
Exits non-zero on a non-2xx response. Standard library only.

Environment:
  DISCORD_WEBHOOK_URL  the #announcements webhook (if empty, this is a no-op)
  REPO                 owner/repo (e.g. turanheydarli/bani.sh)
  TAG                  the release tag (vX.Y.Z)
  PREV_TAG             the previous tag (for the compare link); may be empty
  NOTES                the GitHub Release body
  CONTRIBUTORS         newline-separated contributor names for this release
  FIRST_TIMERS         newline-separated names making their first contribution
"""

import json
import os
import re
import sys
import urllib.error
import urllib.request

GREEN = 2990158   # normal release stripe (#2da44e)
AMBER = 12552960  # breaking-change stripe (#bf8700)
RAW = "https://raw.githubusercontent.com/{repo}/main/assets/{name}"


def env(name, default=""):
    return os.environ.get(name, default).strip()


def nonempty_lines(text):
    return [line for line in text.splitlines() if line.strip()]


def build_payload():
    repo = env("REPO")
    tag = env("TAG")
    prev = env("PREV_TAG")
    notes = os.environ.get("NOTES", "")
    contributors = nonempty_lines(env("CONTRIBUTORS"))
    first_timers = set(nonempty_lines(env("FIRST_TIMERS")))

    release_url = "https://github.com/{}/releases/tag/{}".format(repo, tag)

    def asset(name):
        return RAW.format(repo=repo, name=name)

    breaking = "BREAKING" in notes.upper()

    # Title: "vX.Y.Z - the <theme> release" when the notes carry a Theme: line.
    theme_match = re.search(r"(?im)^\s*Theme:\s*(.+)$", notes)
    if theme_match:
        title = "{} - the {} release".format(tag, theme_match.group(1).strip())
    else:
        title = tag

    # Description: lead with the measured number. Prefer a "Savings:" line or the
    # first line carrying a percentage; otherwise the first paragraph.
    number = ""
    for line in nonempty_lines(notes):
        if line.lower().startswith("savings:") or re.search(r"\d+%", line):
            number = re.sub(r"(?i)^savings:\s*", "", line).strip()
            break
    if number:
        desc = number
    else:
        paragraphs = [p.strip() for p in notes.split("\n\n") if p.strip()]
        desc = paragraphs[0] if paragraphs else "{} is out.".format(tag)
    if len(desc) > 350:
        desc = desc[:347].rstrip() + "..."
    desc += "\n\nRun `banish gain` to see yours."

    fields = []

    # Highlights: up to 5 bullet lines from the notes.
    bullets = []
    for line in notes.splitlines():
        stripped = line.strip()
        if stripped[:2] in ("- ", "* ") or stripped.startswith("\u2022"):
            bullets.append("- " + stripped.lstrip("-*\u2022 ").strip())
            if len(bullets) >= 5:
                break
    if bullets:
        fields.append({"name": "Highlights", "value": "\n".join(bullets)})

    # Shipped by: name every contributor; welcome first-timers.
    if contributors:
        value = ", ".join(contributors)
        welcomed = [c for c in contributors if c in first_timers]
        if welcomed:
            value += "\n\nFirst contribution from " + ", ".join(welcomed) + " - welcome aboard."
        fields.append({"name": "Shipped by", "value": value})

    # Links (zero-width field name keeps the row clean).
    links = ["[Release notes]({})".format(release_url), "[Install](https://bani.sh)"]
    if prev:
        links.append("[Full changelog](https://github.com/{}/compare/{}...{})".format(repo, prev, tag))
    fields.append({"name": "\u200b", "value": " | ".join(links)})

    embed = {
        "author": {"name": "banish", "icon_url": asset("favicon.png")},
        "title": title,
        "url": release_url,
        "color": AMBER if breaking else GREEN,
        "description": desc,
        "fields": fields,
        "thumbnail": {"url": asset("mascot-ghost.png")},
        "footer": {"text": "banish | MIT | bani.sh", "icon_url": asset("favicon.png")},
    }

    payload = {
        "username": "banish-bot",
        "avatar_url": asset("bot-avatar.png"),
        "embeds": [embed],
    }
    if breaking:
        payload["content"] = "@everyone"
    return payload


def main():
    webhook = env("DISCORD_WEBHOOK_URL")
    if not webhook:
        print("DISCORD_WEBHOOK_URL not set; nothing to post.")
        return 0

    data = json.dumps(build_payload()).encode("utf-8")
    request = urllib.request.Request(
        webhook,
        data=data,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(request) as resp:
            code = resp.status
    except urllib.error.HTTPError as err:
        body = err.read().decode("utf-8", "replace")
        print("Discord rejected the post: HTTP {}\n{}".format(err.code, body), file=sys.stderr)
        return 1
    except Exception as err:  # noqa: BLE001 - fail loud on any post error
        print("Failed to post to Discord: {}".format(err), file=sys.stderr)
        return 1

    if not 200 <= code < 300:
        print("Discord returned HTTP {}".format(code), file=sys.stderr)
        return 1
    print("Posted {} to Discord (HTTP {}).".format(env("TAG"), code))
    return 0


if __name__ == "__main__":
    sys.exit(main())
