# GCM Documentation

<p align="center">
  <strong>Git Config Manager — Manage your Git identities with ease</strong>
</p>

**GCM** is a command-line tool that manages multiple complete Git identities — not just `user.name` and `user.email`, but also SSH keys, GPG signing keys, provider tokens, and editor preferences — and switches between them with one command.

---

## For Users

If you are new to GCM or want to learn how to use it effectively, start here.

- **[Quick Start](quick-start.md)** — Get up and running in 5 minutes.
- **[Installation](installation.md)** — Detailed instructions for all platforms.
- **[Requirements](requirements.md)** — System requirements, shell compatibility, platform notes.
- **[Getting Started](getting-started.md)** — Step-by-step first-time setup.
- **[Commands Reference](commands.md)** — A complete reference for every CLI command and flag.
- **[Interactive Guide](interactive-guide.md)** — Every prompt, term, and option explained in plain English.
- **[Configuration](configuration.md)** — Customize GCM with `config.yaml`, profile YAML, and templates.
- **[Provider Integrations](providers.md)** — GitHub, GitLab, and future provider architecture.
- **[Shell Integration](shell-integration.md)** — Auto-switch profiles on `cd`, prompt indicators.
- **[Examples](examples.md)** — Real-world workflows, CI/CD, teams, and scripting.
- **[Migration Guide](migration-guide.md)** — Migrate from manual Git config, shell functions, or other tools.
- **[Developer Onboarding](developer-onboarding.md)** — Team adoption guide, CI/CD, onboarding checklists.
- **[FAQ](faq.md)** — Answers to frequently asked questions.
- **[Troubleshooting](troubleshooting.md)** — Solutions to common problems.
- **[Upgrade & Uninstall](upgrade-uninstall.md)** — Update GCM, remove it cleanly.

---

## For Developers

If you want to contribute to GCM or understand its internals, this section is for you.

- **[Contributing](contributing.md)** — How to report bugs, propose features, and submit pull requests.
- **[Architecture Overview](architecture.md)** — Layers, design patterns, and component responsibilities.
- **[Provider Integrations](providers.md)** — Provider abstraction, token storage, credential helper flow.
- **[Project Structure](project-structure.md)** — A map of every package and file in the codebase.
- **[Data Flow & Diagrams](data-flow.md)** — Trace how key operations work end-to-end.
- **[Dependencies](dependencies.md)** — External modules, standard library usage, and rationale.
- **[Security Model](security.md)** — Threat model, encryption, permissions, and audit logging.
- **[Performance](performance.md)** — Benchmarks, optimizations, and profiling.
- **[Versioning](versioning.md)** — SemVer policy, compatibility guarantees, support lifecycle.
- **[Release Notes](release-notes.md)** — Release history and upgrade paths.
- **[Glossary](glossary.md)** — Definitions for terms used throughout the docs and code.

---

## Quick Links

| I want to…                         | Go to                                                    |
| ---------------------------------- | -------------------------------------------------------- |
| Install GCM                        | [Installation](installation.md)                          |
| Create my first profile            | [Quick Start](quick-start.md)                            |
| See all commands                   | [Commands Reference](commands.md)                        |
| Set up auto-switching              | [Shell Integration](shell-integration.md)                |
| Understand the config file         | [Configuration](configuration.md)                        |
| Fix a problem                      | [Troubleshooting](troubleshooting.md)                    |
| Migrate from manual setup          | [Migration Guide](migration-guide.md)                    |
| Onboard my team                    | [Developer Onboarding](developer-onboarding.md)          |
| Contribute code                    | [Contributing](contributing.md)                          |
| Understand the architecture        | [Architecture](architecture.md)                          |
| See feature demos                  | [Demos](demos/)                                          |

---

## Documentation Map

```
docs/
├── index.md                 ← You are here
│
├── User Guides
│   ├── quick-start.md       — 5-minute setup
│   ├── installation.md      — All platforms
│   ├── getting-started.md   — Detailed first-time walkthrough
│   ├── usage.md             — End-to-end usage guide
│   ├── commands.md          — Every command, flag, and exit code
│   ├── configuration.md     — config.yaml, profile schema, templates
│   ├── providers.md         — GitHub/GitLab provider integrations
│   ├── shell-integration.md — Shell hooks, auto-switching
│   ├── examples.md          — Workflows and recipes
│   ├── migration-guide.md   — Migrate from other setups
│   ├── developer-onboarding.md — Team adoption guide
│   ├── faq.md               — Frequently asked questions
│   ├── troubleshooting.md   — Problem → solution
│   └── upgrade-uninstall.md — Updating and removing GCM
│
├── Developer Guides
│   ├── contributing.md      — How to contribute
│   ├── architecture.md      — Design and patterns
│   ├── project-structure.md — Codebase map
│   ├── data-flow.md         — Operation traces and diagrams
│   ├── dependencies.md      — Module dependencies
│   ├── security.md          — Security model
│   ├── performance.md       — Benchmarks and optimization
│   ├── versioning.md        — Versioning policy
│   ├── release-notes.md     — Release history
│   └── glossary.md          — Term definitions
│
├── Reference
│   └── requirements.md      — System requirements
│
└── sidebar.md               — Navigation sidebar
```
