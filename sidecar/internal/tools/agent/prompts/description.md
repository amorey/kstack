Launch a new agent to handle complex, multi-step tasks. Each agent type has specific capabilities and tools available to it.

Available agent types:
- general-purpose: General-purpose agent for researching complex questions and executing multi-step tasks: listing across namespaces, reading several pods' logs, comparing configurations. (Tools: all of yours except Agent)

When using the Agent tool, specify a subagent_type to select an agent; omitting it starts a general-purpose agent.

## When to use

Reach for this when the task matches an available agent type, or when answering would mean many calls whose output you don't need to keep — delegate it and you keep the conclusion, not the raw output. For a single lookup where you already know the object, file or command, do it directly. Once you've delegated a task, don't also do it yourself — wait for its notification.

The agent starts with no memory of this conversation. It sees the cluster card and your prompt, nothing else, so the prompt is a complete brief: the goal, what you already know, the names involved, and what to return.

- The agent's final report is not shown to the user — relay what matters.
- Your call answers at once, and the agent's report arrives later as a notification. Each call of the agent's that needs the user's approval waits on the user, as yours do.
