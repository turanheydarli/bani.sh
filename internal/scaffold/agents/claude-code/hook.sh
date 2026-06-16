#!/bin/bash
# banish-hook.sh -- Route Bash commands through banish for output compaction.
# Installed by: banish init claude-code
#
# All decision logic lives in 'banish hook': it reads the tool input on stdin,
# checks the command against your Claude Code permission rules, and only
# auto-approves what those rules already allow. Anything else is left for Claude
# Code to prompt you on, exactly as it would without banish.

command -v banish >/dev/null 2>&1 || exit 0
exec banish hook
