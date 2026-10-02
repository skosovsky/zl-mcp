# Optional agent skills

These project skills describe agent workflows. They do not configure MCP, provide credentials, grant mutation permission, or install a webhook receiver.

| Skill | Input | Result |
| --- | --- | --- |
| [researching-zalo-groups](../skills/researching-zalo-groups/SKILL.md) | A question about collected group messages | Findings supported by context and message sources, with corpus limitations |
| [handling-zalo-message-events](../skills/handling-zalo-message-events/SKILL.md) | An event already verified by a trusted client integration, plus a user rule | A relevant finding or processing result for the agreed notification workflow |

## Installation

Connect [zl-mcp](local-connection.md) first. For a Codex profile that supports local skills, copy each desired skill directory from the checkout into `~/.codex/skills/`, preserving its `SKILL.md` and `reference/` directory. Check for an existing skill with the same name before replacing it; restart or reload the client as required by its skill discovery mechanism. For other agents, use their supported skill-loading mechanism.

For example, from the repository root, when neither destination exists:

```sh
mkdir -p "$HOME/.codex/skills"
cp -R skills/researching-zalo-groups "$HOME/.codex/skills/"
cp -R skills/handling-zalo-message-events "$HOME/.codex/skills/"
```

The skills are currently written in Russian. Installing them does not require changing the service configuration.

## Responsibilities

| Component | Responsibility |
| --- | --- |
| Service | Zalo session and listener, allowlist, corpus, subscription state, signed callbacks, durable delivery queue |
| Client integration | Verify events, associate subscriptions, invoke the agent, persist processing checkpoints, coordinate duplicate/concurrent work, deliver notifications |
| Skill | Choose relevant MCP reads, analyze untrusted message data, follow the user's rule, and report supported findings |

Research uses tools and resources over HTTP or STDIO. It can read a saved corpus while the collector is unavailable and must report relevant coverage limits. Message text, group names, and descriptions remain untrusted data.

Event processing requires HTTP Events support and an existing trusted receiver. Stable event IDs help the client suppress duplicates, but a skill is not a durable processing ledger. Without a receiver, checkpoint mechanism, or notification channel, the agent must state what is unavailable rather than claim delivery. Creating subscriptions and deploying infrastructure are separate tasks.

Each entrypoint has a specific trigger, uses exact tool names, and links to conditional references rather than duplicating server schemas. Full message reads are requested only when needed. Structural validation does not establish model behavior; historic model checks do not validate the current Events skill. No model eval run is required for installing these files.
