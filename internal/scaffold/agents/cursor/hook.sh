#!/bin/bash
# banish-hook.sh -- Route Cursor shell commands through banish for compaction.
# Installed by: banish init cursor
#
# Thin delegate: 'banish hook --host cursor' reads the tool input on stdin,
# checks the command against your Cursor permission rules, and emits Cursor's
# preToolUse JSON envelope (or {} when there is nothing to rewrite).

command -v banish >/dev/null 2>&1 || { echo '{}'; exit 0; }
exec banish hook --host cursor
