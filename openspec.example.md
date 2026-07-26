---
# This file reproduces `see`'s default OpenSpec mode as a Markdown
# workflow. It exists only as a reference/template: copy it into your
# workflows_dir (default ~/.config/see/workflows/), rename it, and edit.
# The filename minus ".md" is the workflow name; the frontmatter `name:`
# below is accepted but ignored.
#
# Place a `.md` file in workflows_dir, OR drop the equivalent into the
# `workflows:` sequence in config.yaml. Doing either switches `see` into
# custom-workflow mode for every watched repository, so keep this entry
# around if you want the OpenSpec behavior preserved alongside others.
condition: |-
  for d in openspec/changes/*/; do
    [ -d "$d" ] || continue
    case "$d" in
      openspec/changes/archive/) continue ;;
    esac
    n=${d%/}
    printf %s "${n##*/}"
    exit 0
  done
  exit 1
commit: "see: apply openspec change {change}"
# model: "openai/gpt-5-mini"   # optional; omit to use the agent default
---

Apply the openspec change "{change}": read its proposal and tasks, implement them, run the tests, verify, then archive the change. Sync specs if needed.
