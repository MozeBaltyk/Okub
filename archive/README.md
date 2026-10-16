# Archive

Legacy implementation kept for reference. **Not maintained, not built, not run.**

The original project was an Ansible collection driven by `just` + OpenTofu
(`archive/roles`, `archive/playbooks`, `archive/libvirt`, `archive/scripts`,
`archive/meta`, `archive/justfile`, `archive/galaxy.yml`).

Okub is being refactored into a Go CLI/TUI tool; see [`../docs/design.md`](../docs/design.md).
Anything useful here (templates, install-config/agent-config logic, pfSense
config) should be ported deliberately, not depended on.
